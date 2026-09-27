// Package manage implements the handful of account operations that cannot
// happen through the web UI, because they are needed before anyone can log in
// or at the point when nobody can.
//
// It lives in a package rather than a main so that both entry points can
// share it: the single portable binary dispatches "myapp manage ..." here,
// and cmd/manage builds it as a standalone tool for the container image.
// One implementation, two ways to reach it.
//
// # How it reaches the database
//
// bolt hands its file to exactly one process, so this tool cannot simply open
// the database while the server is running. It takes whichever route is
// available, in this order:
//
//	server running   ask it over the control socket (server/control)
//	server stopped   open the database directly
//
// Every command is written once against the backend interface below and works
// either way, so the same `reissue-login` that rescues a locked-out user on a
// live deployment also works on a fresh install with nothing running yet.
package manage

import (
	errors "errors"
	flag "flag"
	fmt "fmt"
	os "os"
	time "time"

	bootstrap "{{MODULE_PATH}}/server/bootstrap"
	config "{{MODULE_PATH}}/server/config"
	control "{{MODULE_PATH}}/server/control"
	db "{{MODULE_PATH}}/server/db"
	models "{{MODULE_PATH}}/server/models"
)

const usage = `manage -- account administration for {{PROJECT_NAME}}

Usage:
  bootstrap-admin -name "Ada"        create the first admin + login link
  create-login    -name "Grace" -role user
  reissue-login   -user-id 3         fresh link for an existing user
  list-users                         id, name, role, status
  set-disabled    -user-id 3 -disabled=true
  list-keys       [-user-id 3]       API keys, whose they are, when last used
  create-key      -user-id 3 -name "CI" [-role user] [-days 90]
  revoke-key      -key-id abc123     kill one API key now
  paths                              where this app keeps its state

An API key carries a role from the same set users do, so an admin-scoped key
can do what an admin can and a user-scoped one cannot. A key never outranks
the account it belongs to. -days 0 means it never expires; omit -days to use
the server's configured default.

Reachable two ways, identical either way:
  ./{{PROJECT_SLUG}} manage list-users     (the single portable binary)
  ./manage list-users                      (the standalone tool in the image)

These work whether or not the server is running. With the server up the
command is sent to it over the control socket in the app directory; with the
server stopped the database is opened directly.

Every command reads the same environment variables as the server
(DATA_DIR, SECRET_KEY, ...). SECRET_KEY in particular must match, or
the stored records cannot be decrypted.
`

// Usage prints the command list. Exported so an entry point can show it for
// a bare "manage" with no subcommand.
func Usage() {
	fmt.Print( usage )
}

// backend is the small set of operations the commands need. Having both a
// control-socket implementation and a direct-database one behind one
// interface is what keeps each command written a single time -- the
// alternative is two copies of every command that drift apart.
//
// It speaks in control.UserResponse rather than *models.User because that is
// the shape that survives the socket; the direct implementation converts.
type backend interface {
	ListUsers() ( users []control.UserResponse , err error )
	CreateUser( display_name string , role string ) ( user control.UserResponse , err error )
	ReissueLogin( user_id uint64 ) ( user control.UserResponse , err error )
	SetDisabled( user_id uint64 , disabled bool ) ( user control.UserResponse , err error )
	ListAPIKeys( user_id uint64 ) ( keys []control.APIKeyResponse , err error )
	CreateAPIKey( user_id uint64 , name string , role string , expires_in_days *int ) ( key control.APIKeyResponse , err error )
	RevokeAPIKey( key_id string ) ( key control.APIKeyResponse , err error )
	AnyAdminExists() ( result bool , err error )
	Close() ( err error )
	Route() ( description string )
}

// Run executes one subcommand. arguments is everything after the program name
// (for cmd/manage) or after "manage" (for the combined binary), so both
// callers pass the same shape.
func Run( arguments []string ) ( err error ) {
	if len( arguments ) < 1 {
		Usage()
		err = fmt.Errorf( "no command given" )
		return
	}

	cfg , err := config.Load()
	if err != nil {
		err = fmt.Errorf( "config error: %w" , err )
		return
	}

	// "paths" reports configuration only, so it never needs the database and
	// stays useful when everything else is failing.
	command := arguments[ 0 ]
	rest := arguments[ 1: ]
	if command == "paths" {
		err = commandPaths( cfg )
		return
	}

	back , err := connect( cfg )
	if err != nil { return }
	defer back.Close()

	// Say which route this took. Whether a command reached a live server or
	// opened the file itself changes what "it worked" means, and it goes to
	// stderr so piping list-users stays clean.
	fmt.Fprintf( os.Stderr , "[manage] %s\n" , back.Route() )

	switch command {
	case "bootstrap-admin":
		err = commandBootstrapAdmin( back , cfg , rest )
	case "create-login":
		err = commandCreateLogin( back , cfg , rest )
	case "reissue-login":
		err = commandReissueLogin( back , cfg , rest )
	case "list-users":
		err = commandListUsers( back )
	case "set-disabled":
		err = commandSetDisabled( back , rest )
	case "list-keys":
		err = commandListKeys( back , rest )
	case "create-key":
		err = commandCreateKey( back , cfg , rest )
	case "revoke-key":
		err = commandRevokeKey( back , rest )
	default:
		Usage()
		err = fmt.Errorf( "unknown command %q" , command )
	}
	return
}

// connect prefers a running server and falls back to the database file.
//
// The fallback is not a workaround, it is the point: on a fresh install there
// is no server yet, and `bootstrap-admin` has to work before anything else
// exists. The failure message names both routes, because "database is locked"
// on its own sends people hunting for the wrong problem.
func connect( cfg *config.Config ) ( back backend , err error ) {
	client , dial_err := control.Dial( cfg )
	if dial_err == nil {
		back = &controlBackend{ client: client , path: cfg.ControlSocketPath }
		return
	}
	if errors.Is( dial_err , control.ErrNoServer ) == false {
		err = fmt.Errorf( "control socket error: %w" , dial_err )
		return
	}

	store , open_err := db.Open( cfg )
	if open_err != nil {
		if cfg.ControlSocketEnabled {
			err = fmt.Errorf(
				"could not reach a running server on %s, and could not open the database directly: %w\n"+
					"If the server is running, it was started with CONTROL_SOCKET=false; restart it with the socket enabled or stop it first." ,
				cfg.ControlSocketPath , open_err )
			return
		}
		err = fmt.Errorf( "database error (CONTROL_SOCKET is disabled, so the server must be stopped): %w" , open_err )
		return
	}
	back = &directBackend{ store: store , cfg: cfg }
	return
}

// controlBackend sends each command to the running server.
type controlBackend struct {
	client *control.Client
	path   string
}

func ( b *controlBackend ) Route() ( description string ) {
	description = fmt.Sprintf( "running server via %s" , b.path )
	return
}

func ( b *controlBackend ) Close() ( err error ) {
	err = b.client.Close()
	return
}

func ( b *controlBackend ) ListUsers() ( users []control.UserResponse , err error ) {
	users , err = b.client.ListUsers()
	return
}

func ( b *controlBackend ) CreateUser( display_name string , role string ) ( user control.UserResponse , err error ) {
	user , err = b.client.CreateUser( display_name , role )
	return
}

func ( b *controlBackend ) ReissueLogin( user_id uint64 ) ( user control.UserResponse , err error ) {
	user , err = b.client.ReissueLogin( user_id )
	return
}

func ( b *controlBackend ) SetDisabled( user_id uint64 , disabled bool ) ( user control.UserResponse , err error ) {
	user , err = b.client.SetDisabled( user_id , disabled )
	return
}

func ( b *controlBackend ) ListAPIKeys( user_id uint64 ) ( keys []control.APIKeyResponse , err error ) {
	keys , err = b.client.ListAPIKeys( user_id )
	return
}

func ( b *controlBackend ) CreateAPIKey( user_id uint64 , name string , role string , expires_in_days *int ) ( key control.APIKeyResponse , err error ) {
	key , err = b.client.CreateAPIKey( user_id , name , role , expires_in_days )
	return
}

func ( b *controlBackend ) RevokeAPIKey( key_id string ) ( key control.APIKeyResponse , err error ) {
	key , err = b.client.RevokeAPIKey( key_id )
	return
}

// AnyAdminExists is derived from the user list rather than given its own
// endpoint. It is asked once, by bootstrap-admin, and a listing is cheap.
func ( b *controlBackend ) AnyAdminExists() ( result bool , err error ) {
	users , err := b.client.ListUsers()
	if err != nil { return }
	for _ , user := range users {
		if user.Role == models.RoleAdmin && user.Disabled == false {
			result = true
			return
		}
	}
	return
}

// directBackend opens the database itself, for when no server is running.
type directBackend struct {
	store *db.Store
	cfg   *config.Config
}

func ( b *directBackend ) Route() ( description string ) {
	description = fmt.Sprintf( "database directly at %s" , b.cfg.DatabasePath )
	return
}

func ( b *directBackend ) Close() ( err error ) {
	err = b.store.Close()
	return
}

func fromModel( user *models.User , credential string ) ( result control.UserResponse ) {
	result = control.UserResponse{
		ID:          user.ID,
		DisplayName: user.DisplayName,
		Role:        user.Role,
		Disabled:    user.Disabled(),
		Credential:  credential,
	}
	return
}

func ( b *directBackend ) ListUsers() ( users []control.UserResponse , err error ) {
	stored , err := models.ListUsers( b.store )
	if err != nil { return }
	users = []control.UserResponse{}
	for _ , user := range stored {
		users = append( users , fromModel( user , "" ) )
	}
	return
}

func ( b *directBackend ) CreateUser( display_name string , role string ) ( user control.UserResponse , err error ) {
	created , err := models.CreateUser( b.store , display_name , role )
	if err != nil { return }
	credential , err := models.IssueLoginToken( b.store , created.ID , b.cfg.LoginTokenTTL )
	if err != nil { return }
	user = fromModel( created , credential )
	return
}

func ( b *directBackend ) ReissueLogin( user_id uint64 ) ( user control.UserResponse , err error ) {
	found , err := models.GetUser( b.store , user_id )
	if err != nil {
		err = fmt.Errorf( "no user with id %d" , user_id )
		return
	}
	credential , err := models.IssueLoginToken( b.store , found.ID , b.cfg.LoginTokenTTL )
	if err != nil { return }
	user = fromModel( found , credential )
	return
}

func ( b *directBackend ) SetDisabled( user_id uint64 , disabled bool ) ( user control.UserResponse , err error ) {
	found , err := models.GetUser( b.store , user_id )
	if err != nil {
		err = fmt.Errorf( "no user with id %d" , user_id )
		return
	}
	if err = models.SetUserDisabled( b.store , found.ID , disabled ); err != nil { return }
	if disabled {
		// Same reasoning as the admin endpoint and the control handler:
		// revoking access should take effect now, not whenever the cookie
		// happens to expire.
		if err = models.DestroyAllSessionsForUser( b.store , found.ID ); err != nil { return }
	}
	refreshed , err := models.GetUser( b.store , found.ID )
	if err != nil { return }
	user = fromModel( refreshed , "" )
	return
}

// fromKeyModel mirrors control's own conversion so that a listing looks the
// same whether it came over the socket or out of the database.
func fromKeyModel( key *models.APIKey , display_name string , credential string ) ( result control.APIKeyResponse ) {
	result = control.APIKeyResponse{
		ID:          key.ID,
		UserID:      key.UserID,
		DisplayName: display_name,
		Name:        key.Name,
		Role:        key.Role,
		Live:        key.Live(),
		Revoked:     key.Revoked(),
		Expired:     key.Expired(),
		CreatedAt:   key.CreatedAt,
		ExpiresAt:   key.ExpiresAt,
		LastUsedAt:  key.LastUsedAt,
		RevokedAt:   key.RevokedAt,
		Credential:  credential,
	}
	return
}

// displayNames is one pass over the users bucket, rather than a read per key.
func ( b *directBackend ) displayNames() ( names map[ uint64 ]string , err error ) {
	names = map[ uint64 ]string{}
	users , err := models.ListUsers( b.store )
	if err != nil { return }
	for _ , user := range users {
		names[ user.ID ] = user.DisplayName
	}
	return
}

func ( b *directBackend ) ListAPIKeys( user_id uint64 ) ( keys []control.APIKeyResponse , err error ) {
	var stored []*models.APIKey
	if user_id == 0 {
		stored , err = models.ListAPIKeys( b.store )
	} else {
		stored , err = models.ListAPIKeysForUser( b.store , user_id )
	}
	if err != nil { return }

	names , err := b.displayNames()
	if err != nil { return }

	keys = []control.APIKeyResponse{}
	for _ , key := range stored {
		keys = append( keys , fromKeyModel( key , names[ key.UserID ] , "" ) )
	}
	return
}

func ( b *directBackend ) CreateAPIKey( user_id uint64 , name string , role string , expires_in_days *int ) ( key control.APIKeyResponse , err error ) {
	// Refuse rather than mint something that cannot be used: with the feature
	// off the server ignores the Authorization header entirely.
	if b.cfg.APIKeysEnabled == false {
		err = fmt.Errorf( "api keys are disabled on this server (API_KEYS=false)" )
		return
	}

	owner , err := models.GetUser( b.store , user_id )
	if err != nil {
		err = fmt.Errorf( "no user with id %d" , user_id )
		return
	}
	ttl , ok := models.APIKeyTTLFromDays( expires_in_days , b.cfg.APIKeyTTL )
	if ok == false {
		err = fmt.Errorf( "-days must be 0 (never) or up to %d" , models.APIKeyMaxDays )
		return
	}

	credential , created , err := models.IssueAPIKey( b.store , owner.ID , name , role , ttl )
	if err != nil { return }
	key = fromKeyModel( created , owner.DisplayName , credential )
	return
}

func ( b *directBackend ) RevokeAPIKey( key_id string ) ( key control.APIKeyResponse , err error ) {
	found , err := models.GetAPIKey( b.store , key_id )
	if err != nil {
		err = fmt.Errorf( "no api key with id %s" , key_id )
		return
	}
	if err = models.RevokeAPIKey( b.store , found.ID ); err != nil { return }

	refreshed , err := models.GetAPIKey( b.store , found.ID )
	if err != nil { return }
	names , err := b.displayNames()
	if err != nil { return }
	key = fromKeyModel( refreshed , names[ refreshed.UserID ] , "" )
	return
}

func ( b *directBackend ) AnyAdminExists() ( result bool , err error ) {
	result , err = models.AnyAdminExists( b.store )
	return
}

func commandBootstrapAdmin( back backend , cfg *config.Config , arguments []string ) ( err error ) {
	set := flag.NewFlagSet( "bootstrap-admin" , flag.ExitOnError )
	name := set.String( "name" , "" , "display name for the first admin" )
	force := set.Bool( "force" , false , "create another admin even if one exists" )
	set.Parse( arguments )

	if models.ValidDisplayName( *name ) == false {
		err = fmt.Errorf( "-name is required (1-80 characters)" )
		return
	}

	exists , err := back.AnyAdminExists()
	if err != nil { return }
	if exists && *force == false {
		fmt.Println( "An admin already exists. Re-run with -force to create another." )
		return
	}

	user , err := back.CreateUser( *name , models.RoleAdmin )
	if err != nil { return }

	bootstrap.PrintLoginLink( cfg , fmt.Sprintf( "admin #%d %q" , user.ID , user.DisplayName ) , user.Credential )
	return
}

func commandCreateLogin( back backend , cfg *config.Config , arguments []string ) ( err error ) {
	set := flag.NewFlagSet( "create-login" , flag.ExitOnError )
	name := set.String( "name" , "" , "display name" )
	role := set.String( "role" , models.RoleUser , "admin or user" )
	set.Parse( arguments )

	if models.ValidDisplayName( *name ) == false {
		err = fmt.Errorf( "-name is required (1-80 characters)" )
		return
	}
	if models.ValidRole( *role ) == false {
		err = fmt.Errorf( "-role must be admin or user" )
		return
	}

	user , err := back.CreateUser( *name , *role )
	if err != nil { return }

	bootstrap.PrintLoginLink( cfg , fmt.Sprintf( "%s #%d %q" , user.Role , user.ID , user.DisplayName ) , user.Credential )
	return
}

func commandReissueLogin( back backend , cfg *config.Config , arguments []string ) ( err error ) {
	set := flag.NewFlagSet( "reissue-login" , flag.ExitOnError )
	user_id := set.Uint64( "user-id" , 0 , "id of an existing user" )
	set.Parse( arguments )

	user , err := back.ReissueLogin( *user_id )
	if err != nil { return }

	bootstrap.PrintLoginLink( cfg , fmt.Sprintf( "%s #%d %q" , user.Role , user.ID , user.DisplayName ) , user.Credential )
	return
}

func existsNote( path string ) ( note string ) {
	if _ , err := os.Stat( path ); err != nil {
		note = "  (absent -- optional)"
	}
	return
}

// commandPaths answers "where did it put my data", which is otherwise a
// question you can only settle by reading the source or the startup log.
func commandPaths( cfg *config.Config ) ( err error ) {
	fmt.Printf( "%-14s %s\n" , "app dir" , cfg.AppDir )
	fmt.Printf( "%-14s %s\n" , "database" , cfg.DatabasePath )
	fmt.Printf( "%-14s %s\n" , "storage" , cfg.StorageDir )
	fmt.Printf( "%-14s %s%s\n" , "config" , cfg.ConfigFile , existsNote( cfg.ConfigFile ) )

	// Report where the key actually came from, not just where a file would
	// live. In a container the key arrives through the environment and no
	// file exists -- printing the path alone would send someone hunting for
	// a file that is deliberately not there.
	fmt.Printf( "%-14s %s\n" , "secret key" , cfg.SecretKeySource )

	// Whether a server is live decides which route every other command
	// takes, so report it here rather than making someone infer it.
	if cfg.ControlSocketEnabled == false {
		fmt.Printf( "%-14s %s\n" , "control" , "disabled (CONTROL_SOCKET=false)" )
	} else if client , dial_err := control.Dial( cfg ); dial_err == nil {
		client.Close()
		fmt.Printf( "%-14s %s  (server running)\n" , "control" , cfg.ControlSocketPath )
	} else {
		fmt.Printf( "%-14s %s  (no server running)\n" , "control" , cfg.ControlSocketPath )
	}

	fmt.Println( "" )
	fmt.Println( "Override the app dir with APP_DIR=/some/path." )
	if cfg.SecretKeyOnDisk {
		fmt.Println( "Back up the app dir as a unit -- app.db is unreadable without secret.key." )
	} else {
		fmt.Println( "The key is supplied externally and is NOT in the app dir." )
		fmt.Println( "Back up the app dir AND whatever holds that key; either alone is useless." )
	}
	return
}

func commandListUsers( back backend ) ( err error ) {
	users , err := back.ListUsers()
	if err != nil { return }
	if len( users ) == 0 {
		fmt.Println( "No users yet. Create the first admin with: manage bootstrap-admin -name \"Your Name\"" )
		return
	}
	fmt.Printf( "%-6s %-30s %-8s %s\n" , "ID" , "NAME" , "ROLE" , "STATUS" )
	for _ , user := range users {
		status := "active"
		if user.Disabled {
			status = "disabled"
		}
		fmt.Printf( "%-6d %-30s %-8s %s\n" , user.ID , user.DisplayName , user.Role , status )
	}
	return
}

// formatKeyTime keeps the listings narrow. Minute precision is enough for
// "when was this last used" and a full RFC3339 stamp per column would make the
// table wrap on any normal terminal.
func formatKeyTime( value *time.Time , absent string ) ( result string ) {
	result = absent
	if value != nil {
		result = value.Local().Format( "2006-01-02 15:04" )
	}
	return
}

func keyStatus( key control.APIKeyResponse ) ( result string ) {
	switch {
	case key.Revoked:
		result = "revoked"
	case key.Expired:
		result = "expired"
	default:
		result = "active"
	}
	return
}

func commandListKeys( back backend , arguments []string ) ( err error ) {
	set := flag.NewFlagSet( "list-keys" , flag.ExitOnError )
	user_id := set.Uint64( "user-id" , 0 , "only this user's keys (default: everyone's)" )
	set.Parse( arguments )

	keys , err := back.ListAPIKeys( *user_id )
	if err != nil { return }
	if len( keys ) == 0 {
		fmt.Println( "No API keys. Create one with: manage create-key -user-id <id> -name \"CI\"" )
		return
	}

	fmt.Printf( "%-18s %-22s %-6s %-20s %-8s %-16s %s\n" ,
		"KEY ID" , "NAME" , "ROLE" , "OWNER" , "STATUS" , "LAST USED" , "EXPIRES" )
	for _ , key := range keys {
		owner := key.DisplayName
		if owner == "" {
			owner = fmt.Sprintf( "#%d" , key.UserID )
		}
		fmt.Printf( "%-18s %-22s %-6s %-20s %-8s %-16s %s\n" ,
			key.ID , key.Name , key.Role , owner , keyStatus( key ) ,
			formatKeyTime( key.LastUsedAt , "never" ) ,
			formatKeyTime( key.ExpiresAt , "never" ) )
	}
	return
}

// commandCreateKey is how a CI system or a cron job gets a credential without
// anyone signing in. The -role flag is what "mirroring admin and non-admin
// permissions" means in practice: -role admin produces a key that can do what
// an admin can, and the default produces one that cannot.
//
// -days is deliberately three-valued: omitted uses the server's default, 0
// means the key never expires, and a number is that many days.
func commandCreateKey( back backend , cfg *config.Config , arguments []string ) ( err error ) {
	set := flag.NewFlagSet( "create-key" , flag.ExitOnError )
	user_id := set.Uint64( "user-id" , 0 , "id of the account the key belongs to" )
	name := set.String( "name" , "" , "what this key is for, e.g. \"deploy bot\"" )
	role := set.String( "role" , models.RoleUser , "admin or user (never more than the account has)" )
	days := set.Int( "days" , -1 , "lifetime in days; 0 never expires; omit for the server default" )
	set.Parse( arguments )

	if models.ValidAPIKeyName( *name ) == false {
		err = fmt.Errorf( "-name is required (1-80 characters)" )
		return
	}
	if models.ValidRole( *role ) == false {
		err = fmt.Errorf( "-role must be admin or user" )
		return
	}

	// A negative value is the "flag not given" signal, which the API expresses
	// as an absent field rather than a number.
	var expires_in_days *int
	if *days >= 0 {
		expires_in_days = days
	}

	key , err := back.CreateAPIKey( *user_id , *name , *role , expires_in_days )
	if err != nil { return }

	expires := formatKeyTime( key.ExpiresAt , "never" )
	bootstrap.PrintAPIKey( cfg ,
		fmt.Sprintf( "%s-scoped %q for #%d %q" , key.Role , key.Name , key.UserID , key.DisplayName ) ,
		key.Credential , expires )
	return
}

func commandRevokeKey( back backend , arguments []string ) ( err error ) {
	set := flag.NewFlagSet( "revoke-key" , flag.ExitOnError )
	key_id := set.String( "key-id" , "" , "id of the key to revoke (from list-keys)" )
	set.Parse( arguments )

	if *key_id == "" {
		err = fmt.Errorf( "-key-id is required (see: manage list-keys)" )
		return
	}

	key , err := back.RevokeAPIKey( *key_id )
	if err != nil { return }
	fmt.Printf( "api key %s (%q, owner #%d) revoked -- it stops working immediately\n" , key.ID , key.Name , key.UserID )
	return
}

func commandSetDisabled( back backend , arguments []string ) ( err error ) {
	set := flag.NewFlagSet( "set-disabled" , flag.ExitOnError )
	user_id := set.Uint64( "user-id" , 0 , "id of an existing user" )
	disabled := set.Bool( "disabled" , true , "true to disable, false to re-enable" )
	set.Parse( arguments )

	user , err := back.SetDisabled( *user_id , *disabled )
	if err != nil { return }
	fmt.Printf( "user #%d (%s) disabled=%v\n" , user.ID , user.DisplayName , user.Disabled )
	return
}
