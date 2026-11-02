package config

// The secret key, in its own file.
//
// It used to be server.secret_key in config.yaml. It moved out so that
// config.yaml could be edited while the server runs -- by the owner's
// Settings screen, by hand, by `config set` -- and mounted writable into the
// container, without any of that ever being able to read or replace the key.
// secret.key is written once and then only read.
//
// It sits beside config.yaml, wherever that is:
//
//	./config/secret.key                  a source checkout
//	~/.config/<slug>/secret.key          a portable binary
//	/app/config/secret.key               the container, mounted read-only

import (
	errors "errors"
	fmt "fmt"
	os "os"
	filepath "path/filepath"
	runtime "runtime"
	strings "strings"

	encryption "{{MODULE_PATH}}/server/encryption"
)

// SecretPathFor is the secret.key that goes with a config.yaml: the one in
// the same folder. One rule for a checkout, a portable install and the
// container means there is nothing else to configure, and nothing to get
// wrong when moving a deployment.
func SecretPathFor( config_path string ) ( path string ) {
	path = filepath.Join( filepath.Dir( config_path ) , SecretFileName )
	return
}

// GenerateSecret is a fresh key: 32 random bytes as 64 hex characters.
func GenerateSecret() ( key string ) {
	key = encryption.GenerateKeyHex()
	return
}

// ReadSecret reads and checks secret.key.
func ReadSecret( path string ) ( key string , err error ) {
	raw , err := os.ReadFile( path )
	if err != nil { return }
	key = strings.TrimSpace( string( raw ) )
	err = CheckSecretKey( key , path )
	return
}

// WriteSecret creates secret.key, refusing to overwrite one: an existing key
// is what existing data was written with. The key is read back before this
// returns, so a full disk or a lying filesystem is found now rather than the
// next time the server needs the key.
func WriteSecret( path string , key string ) ( err error ) {
	if err = CheckSecretKey( key , "the new secret key" ); err != nil { return }
	if err = os.MkdirAll( filepath.Dir( path ) , 0o700 ); err != nil {
		err = fmt.Errorf( "could not create %s: %w" , filepath.Dir( path ) , err )
		return
	}
	handle , err := os.OpenFile( path , os.O_WRONLY|os.O_CREATE|os.O_EXCL , 0o600 )
	if err != nil {
		err = fmt.Errorf( "could not create %s: %w" , path , err )
		return
	}
	_ , err = handle.WriteString( key + "\n" )
	if err == nil {
		err = handle.Sync()
	}
	if close_err := handle.Close(); err == nil {
		err = close_err
	}
	if err != nil {
		os.Remove( path )
		err = fmt.Errorf( "could not write %s: %w" , path , err )
		return
	}
	syncDir( filepath.Dir( path ) )
	written , err := ReadSecret( path )
	if err == nil && written != key {
		err = fmt.Errorf( "%s does not read back as the key just written to it" , path )
	}
	return
}

// MigrationResult says what MigrateLegacySecret did.
type MigrationResult struct {
	// Moved is true when the key was copied from config.yaml into a new
	// secret.key.
	Moved bool
	// Removed is true when server.secret_key's line was taken out of
	// config.yaml -- after Moved, or because an earlier move was interrupted
	// after writing secret.key.
	Removed bool
	SecretFile string
}

// MigrateLegacySecret moves server.secret_key out of a config.yaml written
// before secret.key existed. It is safe to run any number of times, and does
// nothing to a file that has no key in it.
//
// The order is what makes it safe to interrupt: secret.key is written and
// read back first, and only then is the line removed from config.yaml. Stop
// it in between and the next run finds both holding the same key and just
// removes the line. Find them holding different keys and it stops -- which
// one encrypted the data is not something to guess.
func MigrateLegacySecret( config_path string ) ( result MigrationResult , err error ) {
	result.SecretFile = SecretPathFor( config_path )
	raw , err := os.ReadFile( config_path )
	if err != nil { return }
	file , err := decodeOverDefaults( raw , config_path )
	if err != nil { return }
	legacy := strings.TrimSpace( file.Server.SecretKey )
	if legacy == "" { return }
	if err = CheckSecretKey( legacy , "server.secret_key in " + config_path ); err != nil { return }

	if isFile( result.SecretFile ) == false {
		write_err := WriteSecret( result.SecretFile , legacy )
		switch {
		case write_err == nil:
			result.Moved = true
		case errors.Is( write_err , os.ErrExist ):
			// Another process got there first; compare with what it wrote.
		default:
			err = write_err
			return
		}
	}
	if result.Moved == false {
		var existing string
		if existing , err = ReadSecret( result.SecretFile ); err != nil { return }
		if existing != legacy {
			err = fmt.Errorf(
				"%s has server.secret_key, and %s holds a different key.\n"+
					"  Refusing to guess which one encrypted your data. Keep the right one in %s\n"+
					"  and delete the secret_key line from %s." ,
				config_path , result.SecretFile , result.SecretFile , config_path )
			return
		}
	}

	edited , err := EditYAML( raw , nil , []string{ "server.secret_key" } , nil )
	if err != nil {
		err = fmt.Errorf( "the key is safe in %s, but removing server.secret_key from %s failed (%w) -- delete that line by hand" , result.SecretFile , config_path , err )
		return
	}
	after , err := decodeOverDefaults( edited , config_path )
	if err == nil && after.Server.SecretKey != "" {
		err = errors.New( "server.secret_key is still set after removing it" )
	}
	if err != nil {
		err = fmt.Errorf( "the key is safe in %s, but removing server.secret_key from %s failed (%w) -- delete that line by hand" , result.SecretFile , config_path , err )
		return
	}
	if err = writeConfigFile( config_path , edited , false , "" ); err != nil { return }
	result.Removed = true
	return
}

// secretState is what resolveSecret settled on.
type secretState struct {
	Key      string
	Created  bool
	Migrated bool
	// Rewrote is true when config.yaml was changed (the key's line removed).
	Rewrote bool
	Warning string
}

// resolveSecret finds the key for the server to run on.
//
//   - secret.key exists: that is the key. A server.secret_key still in
//     config.yaml must match it (an interrupted migration) and is removed;
//     a different one stops everything.
//   - server.secret_key and no secret.key: an older config.yaml. Move it.
//   - neither: a fresh install gets a new key. One with a database does not
//     -- a new key cannot read it, so the old one has to come back.
//
// Inside the container nothing is written: config/ belongs to the host, and
// docker/run.sh does the move there before it starts anything.
func resolveSecret( file *File , config_path string , database_path string ) ( state secretState , err error ) {
	secret_path := SecretPathFor( config_path )
	legacy := strings.TrimSpace( file.Server.SecretKey )

	if legacy != "" {
		if InContainer() {
			if isFile( secret_path ) == false {
				err = fmt.Errorf( "%s still holds server.secret_key and there is no %s -- run docker/run.sh on the host, which moves the key into config/secret.key and mounts it" , config_path , secret_path )
				return
			}
			if state.Key , err = ReadSecret( secret_path ); err != nil { return }
			if state.Key != legacy {
				err = fmt.Errorf( "%s has server.secret_key, and %s holds a different key -- refusing to guess which one encrypted your data" , config_path , secret_path )
				return
			}
			state.Warning = fmt.Sprintf( "%s still holds server.secret_key (the same key as %s); docker/run.sh removes it" , config_path , secret_path )
			return
		}
		var moved MigrationResult
		if moved , err = MigrateLegacySecret( config_path ); err != nil { return }
		state.Migrated = moved.Moved
		state.Rewrote = moved.Removed
		state.Key , err = ReadSecret( secret_path )
		return
	}

	if isFile( secret_path ) {
		state.Key , err = ReadSecret( secret_path )
		return
	}
	if InContainer() {
		err = fmt.Errorf( "no secret key at %s -- docker/run.sh mounts the project's config/secret.key there" , secret_path )
		return
	}
	if isFile( database_path ) {
		err = fmt.Errorf(
			"no secret key at %s, and %s exists.\n"+
				"  Restore secret.key from your backup -- a new key cannot read existing data." , secret_path , database_path )
		return
	}
	state.Key = GenerateSecret()
	if err = WriteSecret( secret_path , state.Key ); err != nil { return }
	state.Created = true
	return
}

// decodeOverDefaults is Parse without Validate: the file's values over the
// defaults, strictly decoded. The migration uses it because moving the key
// must not depend on every other setting being right.
func decodeOverDefaults( raw []byte , label string ) ( file *File , err error ) {
	file , err = Defaults()
	if err != nil { return }
	if err = decodeStrict( raw , file ); err != nil {
		err = fmt.Errorf( "%s: %w" , label , err )
	}
	return
}

// syncDir makes a rename or a new file in dir durable. Windows has no way to
// open a directory for this, and does not need it.
func syncDir( dir string ) {
	if runtime.GOOS == "windows" { return }
	if handle , err := os.Open( dir ); err == nil {
		handle.Sync()
		handle.Close()
	}
}
