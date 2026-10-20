// Package config is the single source of truth for everything this app can be
// told to do differently: the server, the Docker container, and the portable
// build. All of it lives in one config.yaml.
//
// Nothing reads environment variables. A setting that could come from the
// environment, a file, or a default has to be hunted for in three places;
// one that can only come from config.yaml is answered by reading one file.
// The shell scripts follow the same rule -- they ask this package for their
// values (`config shell`, see command.go) rather than parsing YAML themselves,
// so there is exactly one parser and one set of validation rules.
//
// Defaults are not written in Go. They live in defaults.yaml, which is
// embedded, documents every key, and is what `config init` writes out. A
// config.yaml is decoded on top of it, strictly: an unknown key is an error,
// because a typo that is silently ignored is a setting that silently is not
// applied.
package config

import (
	bytes "bytes"
	_ "embed"
	errors "errors"
	fmt "fmt"
	io "io"
	net "net"
	os "os"
	filepath "path/filepath"
	regexp "regexp"
	runtime "runtime"
	strings "strings"
	time "time"

	yaml "gopkg.in/yaml.v3"

	encryption "{{MODULE_PATH}}/server/encryption"
)

// AppSlug names the application on disk. It is what turns into
// ~/.config/<slug>/, so it is deliberately the lowercase, filesystem-safe
// form of the project name rather than the display name. It is a constant
// rather than a setting because it is how config.yaml itself is found.
const AppSlug = "{{PROJECT_SLUG}}"

// Filenames this app uses. Everything it persists lives under the app
// directory -- see DefaultAppDir for why.
const (
	ConfigFileName    = "config.yaml"
	DatabaseFileName  = "app.db"
	ControlSocketName = "control.sock"
	StorageDirName    = "storage"
	LanguageFileName  = "language.yaml"
)

//go:embed defaults.yaml
var defaultsYAML []byte

// ---------------------------------------------------------------------------
// config.yaml, section by section
// ---------------------------------------------------------------------------

// File mirrors config.yaml. Every section is decoded strictly, including the
// docker and build sections the server itself never reads: validating them
// here is what lets dockerRun.sh and build.sh fail on a typo before they
// start anything.
type File struct {
	App    AppSection    `yaml:"app"`
	Server ServerSection `yaml:"server"`
	Auth   AuthSection   `yaml:"auth"`
	Docker DockerSection `yaml:"docker"`
	Build  BuildSection  `yaml:"build"`
}

type AppSection struct {
	Name string `yaml:"name"`
}

type ServerSection struct {
	Host                   string `yaml:"host"`
	Port                   int    `yaml:"port"`
	AppDir                 string `yaml:"app_dir"`
	StaticDir              string `yaml:"static_dir"`
	LanguageFile           string `yaml:"language_file"`
	SecretKey              string `yaml:"secret_key"`
	EncryptAtRest          bool   `yaml:"encrypt_at_rest"`
	SecureCookies          bool   `yaml:"secure_cookies"`
	CookieName             string `yaml:"cookie_name"`
	TrustProxy             bool     `yaml:"trust_proxy"`
	TrustedProxies         []string `yaml:"trusted_proxies"`
	ProxyHeader            string   `yaml:"proxy_header"`
	BodyLimitBytes         int    `yaml:"body_limit_bytes"`
	RateLimitMax           int    `yaml:"rate_limit_max"`
	RateLimitWindowSeconds int    `yaml:"rate_limit_window_seconds"`
	ControlSocket          bool   `yaml:"control_socket"`
}

type AuthSection struct {
	SessionTTLSeconds    int  `yaml:"session_ttl_seconds"`
	LoginTokenTTLSeconds int  `yaml:"login_token_ttl_seconds"`
	APIKeys              bool `yaml:"api_keys"`
	APIKeyTTLSeconds     int  `yaml:"api_key_ttl_seconds"`
}

type DockerSection struct {
	Image       string `yaml:"image"`
	Container   string `yaml:"container"`
	BindAddress string `yaml:"bind_address"`
	HostPort    int    `yaml:"host_port"`
	DataDir     string `yaml:"data_dir"`
	Restart     string `yaml:"restart"`
	Memory      string `yaml:"memory"`
	CPUs        string `yaml:"cpus"`
	PidsLimit   int    `yaml:"pids_limit"`
	TmpfsSize   string `yaml:"tmpfs_size"`
}

type BuildSection struct {
	BinaryName string   `yaml:"binary_name"`
	OutputDir  string   `yaml:"output_dir"`
	Targets    []string `yaml:"targets"`
}

// ---------------------------------------------------------------------------
// the resolved configuration the server runs on
// ---------------------------------------------------------------------------

// Config is config.yaml after resolution: paths made absolute, seconds turned
// into durations, container overrides applied. The rest of the server reads
// this, never File, so a change to the file's shape stays inside this package.
type Config struct {
	ProjectName string

	// ConfigFile is the config.yaml this was loaded from. It also holds the
	// secret key, which is why it is reported wherever backups are mentioned.
	ConfigFile string

	// ConfigCreated is true when this run wrote ConfigFile because none
	// existed. The startup log says so, since a fresh key was generated.
	ConfigCreated bool

	// ConfigWarning is non-empty when ConfigFile is readable by other users.
	// It is a warning rather than an error so a misconfigured mode never
	// stops a server that was working, but it does get logged on every start.
	ConfigWarning string

	// AppDir is the one directory this application owns: the database, the
	// control socket, and any uploaded or generated files.
	//
	// Defaulting to ~/.config/<slug> rather than ./data matters for the
	// single-file build: a binary run from a Downloads folder, a USB stick,
	// or a different working directory each time still finds the same state.
	AppDir       string
	DatabasePath string

	// ControlSocketPath is the unix socket the running server listens on so
	// that other processes can act on the database without opening it.
	//
	// bolt allows exactly one process to hold the database file, so a second
	// process cannot simply open it while the server is up. Rather than
	// fighting that, the server owns the database and everything else --
	// the manage CLI, a cron job, a sidecar -- asks the server over this
	// socket. One process holds the secret key, and there is no second copy
	// of the auth checks to keep in step.
	//
	// server.control_socket: false switches it off. Then manage only works
	// while the server is stopped.
	ControlSocketPath    string
	ControlSocketEnabled bool

	// StorageDir is for application files -- uploads, exports, generated
	// artifacts. Nothing in the base template writes here; it exists so that
	// features added later have an obvious, backed-up place to put files
	// instead of inventing one.
	StorageDir string

	// StaticDir and LanguageFile are checked on disk first; if either is
	// missing, the copy embedded in the binary is used instead (see
	// server/assets). That is what lets one build serve a live directory
	// during development and run as a single self-contained file in
	// production, with no flag to remember.
	StaticDir    string
	LanguageFile string

	Host string
	Port int

	// SecureCookies must stay true anywhere a real browser will talk to this
	// over the internet. It is only false for plain-http://localhost work,
	// because a Secure cookie is silently dropped over http and the login
	// flow would appear to succeed and then do nothing.
	SecureCookies bool
	CookieName    string

	SessionTTL    time.Duration
	LoginTokenTTL time.Duration

	// APIKeysEnabled decides whether the Authorization: Bearer header is
	// believed at all, and whether the endpoints that mint keys exist.
	// Turning it off stops existing keys from authenticating too -- the rows
	// stay, so switching it back on restores them.
	APIKeysEnabled bool

	// APIKeyTTL is how long a new key lasts when the request does not say.
	// The default is bounded so that a forgotten key eventually stops being
	// a live credential.
	APIKeyTTL time.Duration

	// SecretKey encrypts session cookie payloads and, when EncryptAtRest is
	// on, every value written to bolt. Losing it means losing the database
	// contents.
	SecretKey     string
	EncryptAtRest bool

	RateLimitMax    int
	RateLimitWindow time.Duration
	BodyLimit       int

	// TrustProxy tells fiber to believe ProxyHeader -- but only on a
	// connection whose peer address is in TrustedProxies. Fiber with
	// TrustProxy on and no allowlist trusts nobody, so every visitor behind
	// the proxy shares the proxy's one rate-limit budget; with the allowlist
	// and nothing else, any client could still name its own IP. Both halves
	// are required, and Validate refuses one without the other.
	TrustProxy     bool
	TrustedProxies []string
	ProxyHeader    string
}

// layout is what the container image pins, regardless of config.yaml. See
// layout_container.go; outside the image every field is empty and nothing is
// pinned.
type layout struct {
	ConfigFile string
	AppDir     string
	Host       string
}

// InContainer reports whether this binary was built for the container image.
func InContainer() ( result bool ) {
	result = pinned.ConfigFile != ""
	return
}

// ---------------------------------------------------------------------------
// where things live
// ---------------------------------------------------------------------------

// DefaultAppDir is ~/.config/<slug>. Everything lands in one directory rather
// than being split across XDG's config/data/state roots: this app's state is
// small, and one directory to back up is worth more here than strict
// adherence to a layout designed for larger, more separable applications.
func DefaultAppDir() ( dir string ) {
	if home , err := os.UserHomeDir(); err == nil && home != "" {
		dir = filepath.Join( home , ".config" , AppSlug )
		return
	}
	// No home directory at all (some minimal environments). Fall back to the
	// working directory rather than failing to start.
	dir = filepath.Join( "." , AppSlug + "-data" )
	return
}

// Locate finds the config.yaml in use: the pinned path inside the container
// image, otherwise ./config.yaml, otherwise the one in the default app
// directory. When none exists, path is where one would be created.
//
// ./config.yaml comes first because that is a source checkout, which is also
// where dockerRun.sh and build.sh read it. A portable binary run from
// anywhere else falls through to ~/.config/<slug>/ and finds the same file
// every time.
func Locate() ( path string , exists bool ) {
	if pinned.ConfigFile != "" {
		path = pinned.ConfigFile
		exists = isFile( path )
		return
	}
	if local , err := filepath.Abs( ConfigFileName ); err == nil && isFile( local ) {
		path , exists = local , true
		return
	}
	path = filepath.Join( DefaultAppDir() , ConfigFileName )
	if absolute , err := filepath.Abs( path ); err == nil {
		path = absolute
	}
	exists = isFile( path )
	return
}

func isFile( path string ) ( result bool ) {
	info , err := os.Stat( path )
	result = err == nil && info.Mode().IsRegular()
	return
}

// resolvePath makes a config path absolute against the directory holding
// config.yaml, so the same file means the same thing whatever directory the
// process was started from.
func resolvePath( base string , value string ) ( result string ) {
	result = value
	if result == "" || filepath.IsAbs( result ) {
		return
	}
	result = filepath.Join( base , result )
	return
}

// ---------------------------------------------------------------------------
// reading and validating
// ---------------------------------------------------------------------------

func decodeStrict( raw []byte , out *File ) ( err error ) {
	decoder := yaml.NewDecoder( bytes.NewReader( raw ) )
	decoder.KnownFields( true )
	err = decoder.Decode( out )
	if errors.Is( err , io.EOF ) {
		// An empty file is a valid file that sets nothing.
		err = nil
	}
	return
}

// Defaults is defaults.yaml on its own: the configuration a missing key falls
// back to.
func Defaults() ( file *File , err error ) {
	file = &File{}
	if err = decodeStrict( defaultsYAML , file ); err != nil {
		err = fmt.Errorf( "embedded defaults.yaml is invalid: %w" , err )
	}
	return
}

// Parse decodes a config.yaml over the defaults and validates the result.
// It does not require a secret key -- build.sh reads the build section of a
// checkout that may not have one yet. Load and `config check` do.
func Parse( raw []byte , label string ) ( file *File , err error ) {
	file , err = Defaults()
	if err != nil { return }
	if err = decodeStrict( raw , file ); err != nil {
		err = fmt.Errorf( "%s: %w" , label , err )
		return
	}
	if err = file.Validate(); err != nil {
		err = fmt.Errorf( "%s: %w" , label , err )
	}
	return
}

// ReadFile reads and validates one config.yaml without touching anything else
// on disk.
func ReadFile( path string ) ( file *File , err error ) {
	raw , err := os.ReadFile( path )
	if err != nil { return }
	file , err = Parse( raw , path )
	return
}

// Current is the file the server would use, or the defaults when there is no
// file yet. It never writes anything, which is what the shell-facing commands
// and the healthcheck need.
func Current() ( file *File , path string , exists bool , err error ) {
	path , exists = Locate()
	if exists {
		file , err = ReadFile( path )
		return
	}
	file , err = Defaults()
	return
}

var (
	imagePattern     = regexp.MustCompile( `^[a-z0-9]([a-z0-9._-]*[a-z0-9])?(:[0-9]+)?(/[a-z0-9]([a-z0-9._-]*[a-z0-9])?)*$` )
	containerPattern = regexp.MustCompile( `^[a-zA-Z0-9][a-zA-Z0-9_.-]*$` )
	restartPattern   = regexp.MustCompile( `^(no|always|unless-stopped|on-failure(:[0-9]+)?)$` )
	sizePattern      = regexp.MustCompile( `^[0-9]+[bkmgBKMG]?$` )
	cpusPattern      = regexp.MustCompile( `^[0-9]+(\.[0-9]+)?$` )
	targetPattern    = regexp.MustCompile( `^[a-z0-9]+/[a-z0-9]+$` )
	binaryPattern    = regexp.MustCompile( `^[A-Za-z0-9][A-Za-z0-9._-]*$` )
	secretPattern    = regexp.MustCompile( `^[0-9a-fA-F]{64}$` )
)

// Validate rejects values that would fail later in a confusing way -- a port
// out of range, a Docker size the daemon would refuse, a build target go
// cannot parse. The docker and build values also end up in shell commands,
// so they are held to strict patterns rather than trusted.
func ( file *File ) Validate() ( err error ) {
	var problems []string
	check := func( ok bool , format string , values ...any ) {
		if ok == false {
			problems = append( problems , fmt.Sprintf( format , values... ) )
		}
	}
	path_ok := func( value string ) ( result bool ) {
		result = strings.TrimSpace( value ) != "" && strings.ContainsAny( value , ":\n\r" ) == false
		return
	}

	check( strings.TrimSpace( file.App.Name ) != "" , "app.name must not be empty" )

	server := file.Server
	check( strings.TrimSpace( server.Host ) != "" , "server.host must not be empty" )
	check( server.Port >= 1 && server.Port <= 65535 , "server.port must be 1-65535, got %d" , server.Port )
	check( strings.TrimSpace( server.CookieName ) != "" , "server.cookie_name must not be empty" )
	check( strings.TrimSpace( server.ProxyHeader ) != "" , "server.proxy_header must not be empty" )
	check( server.TrustProxy == false || len( server.TrustedProxies ) > 0 ,
		"server.trust_proxy is true but server.trusted_proxies is empty -- list the address or CIDR range your reverse proxy connects from" )
	for _ , entry := range server.TrustedProxies {
		check( validProxyEntry( entry ) , "server.trusted_proxies entry %q must be an IP address or a CIDR range like 10.0.0.0/8" , entry )
		check( entry != "0.0.0.0/0" && entry != "::/0" , "server.trusted_proxies entry %q trusts every address, which lets any client forge its own IP" , entry )
	}
	check( server.BodyLimitBytes > 0 , "server.body_limit_bytes must be positive" )
	check( server.RateLimitMax > 0 , "server.rate_limit_max must be positive" )
	check( server.RateLimitWindowSeconds > 0 , "server.rate_limit_window_seconds must be positive" )

	auth := file.Auth
	check( auth.SessionTTLSeconds > 0 , "auth.session_ttl_seconds must be positive" )
	check( auth.LoginTokenTTLSeconds > 0 , "auth.login_token_ttl_seconds must be positive" )
	check( auth.APIKeyTTLSeconds > 0 , "auth.api_key_ttl_seconds must be positive" )

	docker := file.Docker
	check( imagePattern.MatchString( docker.Image ) , "docker.image %q is not a valid lowercase image name" , docker.Image )
	check( containerPattern.MatchString( docker.Container ) , "docker.container %q is not a valid container name" , docker.Container )
	check( validBindAddress( docker.BindAddress ) , "docker.bind_address %q must be an IP address" , docker.BindAddress )
	check( docker.HostPort >= 1 && docker.HostPort <= 65535 , "docker.host_port must be 1-65535, got %d" , docker.HostPort )
	check( path_ok( docker.DataDir ) , "docker.data_dir must be a path with no ':' in it" )
	check( restartPattern.MatchString( docker.Restart ) , "docker.restart must be no, always, unless-stopped or on-failure[:N], got %q" , docker.Restart )
	check( sizePattern.MatchString( docker.Memory ) , "docker.memory must look like 256m, got %q" , docker.Memory )
	check( cpusPattern.MatchString( docker.CPUs ) , "docker.cpus must be a number like 1 or 0.5, got %q" , docker.CPUs )
	check( docker.PidsLimit > 0 , "docker.pids_limit must be positive" )
	check( sizePattern.MatchString( docker.TmpfsSize ) , "docker.tmpfs_size must look like 16m, got %q" , docker.TmpfsSize )

	build := file.Build
	check( binaryPattern.MatchString( build.BinaryName ) , "build.binary_name %q must be a plain file name" , build.BinaryName )
	check( path_ok( build.OutputDir ) , "build.output_dir must be a path with no ':' in it" )
	check( len( build.Targets ) > 0 , "build.targets must list at least one os/arch" )
	for _ , target := range build.Targets {
		check( targetPattern.MatchString( target ) , "build.targets entry %q must look like linux/amd64" , target )
	}

	if len( problems ) > 0 {
		err = errors.New( strings.Join( problems , "; " ) )
	}
	return
}

func validProxyEntry( value string ) ( result bool ) {
	if strings.Contains( value , "/" ) {
		_ , _ , err := net.ParseCIDR( value )
		result = err == nil
		return
	}
	result = net.ParseIP( value ) != nil
	return
}

func validBindAddress( value string ) ( result bool ) {
	result = net.ParseIP( value ) != nil
	return
}

// CheckSecretKey is separate from Validate because only a process that will
// actually use the key needs one.
//
// An empty key is refused rather than filled in. A file that already exists
// was created with a key, so an empty one means it was removed -- and
// quietly generating a replacement would make every existing record
// permanently unreadable.
func CheckSecretKey( file *File , path string ) ( err error ) {
	key := strings.TrimSpace( file.Server.SecretKey )
	if key == "" {
		err = fmt.Errorf(
			"server.secret_key is empty in %s\n"+
				"  If this app already has data, put back the key it was written with -- a new key cannot read it.\n"+
				"  For a fresh install, generate one with: openssl rand -hex 32" , path )
		return
	}
	if secretPattern.MatchString( key ) == false {
		err = fmt.Errorf( "server.secret_key in %s must be 64 hex characters (32 bytes), got %d characters" , path , len( key ) )
	}
	return
}

// permissionWarning flags a config.yaml anyone on the machine can read. The
// file holds the secret key, so that is everyone's copy of it.
func permissionWarning( path string ) ( warning string ) {
	if runtime.GOOS == "windows" {
		return
	}
	info , err := os.Stat( path )
	if err != nil { return }
	if info.Mode().Perm() & 0o004 != 0 {
		warning = fmt.Sprintf( "%s is readable by every user (mode %04o) and holds the secret key -- chmod 600 it" , path , info.Mode().Perm() )
	}
	return
}

// ---------------------------------------------------------------------------
// creating a config.yaml
// ---------------------------------------------------------------------------

var secretKeyLine = regexp.MustCompile( `(?m)^(\s*secret_key:\s*)""` )

// NewFileContent is defaults.yaml with a freshly generated secret key -- the
// whole file, comments included, so the result documents itself.
func NewFileContent() ( content []byte , err error ) {
	matches := secretKeyLine.FindAllIndex( defaultsYAML , -1 )
	if len( matches ) != 1 {
		err = fmt.Errorf( "embedded defaults.yaml must contain exactly one `secret_key: \"\"` line, found %d" , len( matches ) )
		return
	}
	key := encryption.GenerateKeyHex()
	content = secretKeyLine.ReplaceAll( defaultsYAML , []byte( `${1}"` + key + `"` ) )
	return
}

// WriteNew creates path from NewFileContent, refusing to overwrite: an
// existing config.yaml holds a key that existing data depends on.
func WriteNew( path string ) ( err error ) {
	content , err := NewFileContent()
	if err != nil { return }
	// 0700 because the secret key lives in here.
	if err = os.MkdirAll( filepath.Dir( path ) , 0o700 ); err != nil {
		err = fmt.Errorf( "could not create %s: %w" , filepath.Dir( path ) , err )
		return
	}
	handle , err := os.OpenFile( path , os.O_WRONLY|os.O_CREATE|os.O_EXCL , 0o600 )
	if err != nil {
		err = fmt.Errorf( "could not create %s: %w" , path , err )
		return
	}
	defer handle.Close()
	_ , err = handle.Write( content )
	return
}

// ---------------------------------------------------------------------------

// Load finds config.yaml (creating it with a fresh key if there is none),
// validates it, and makes sure the app directory exists. The server and the
// manage CLI both start here.
func Load() ( cfg *Config , err error ) {
	path , exists := Locate()
	created := false
	if exists == false {
		if InContainer() {
			err = fmt.Errorf( "no config at %s -- dockerRun.sh mounts config.yaml there" , path )
			return
		}
		if err = WriteNew( path ); err != nil { return }
		created = true
	}

	file , err := ReadFile( path )
	if err != nil { return }
	if err = CheckSecretKey( file , path ); err != nil { return }

	base := filepath.Dir( path )

	app_dir := DefaultAppDir()
	if file.Server.AppDir != "" {
		app_dir = resolvePath( base , file.Server.AppDir )
	}
	if pinned.AppDir != "" {
		app_dir = pinned.AppDir
	}
	if absolute , abs_err := filepath.Abs( app_dir ); abs_err == nil {
		app_dir = absolute
	}

	// 0700 because the database lives in here.
	if mkdir_err := os.MkdirAll( app_dir , 0o700 ); mkdir_err != nil {
		err = fmt.Errorf( "could not create app directory %q: %w" , app_dir , mkdir_err )
		return
	}
	storage_dir := filepath.Join( app_dir , StorageDirName )
	if mkdir_err := os.MkdirAll( storage_dir , 0o700 ); mkdir_err != nil {
		err = fmt.Errorf( "could not create storage directory %q: %w" , storage_dir , mkdir_err )
		return
	}

	// An operator can drop a language.yaml into the app directory to reword
	// the UI without rebuilding. Failing that, the one beside config.yaml (a
	// source checkout) is used, and failing that the embedded copy.
	language_file := resolvePath( base , file.Server.LanguageFile )
	if language_file == "" {
		language_file = filepath.Join( app_dir , LanguageFileName )
		if isFile( language_file ) == false {
			language_file = filepath.Join( base , LanguageFileName )
		}
	}

	host := file.Server.Host
	if pinned.Host != "" {
		host = pinned.Host
	}

	cfg = &Config{
		ProjectName: file.App.Name,

		ConfigFile:    path,
		ConfigCreated: created,
		ConfigWarning: permissionWarning( path ),

		AppDir:       app_dir,
		DatabasePath: filepath.Join( app_dir , DatabaseFileName ),

		ControlSocketPath:    filepath.Join( app_dir , ControlSocketName ),
		ControlSocketEnabled: file.Server.ControlSocket,
		StorageDir:           storage_dir,

		StaticDir:    resolvePath( base , file.Server.StaticDir ),
		LanguageFile: language_file,

		Host: host,
		Port: file.Server.Port,

		SecureCookies: file.Server.SecureCookies,
		CookieName:    file.Server.CookieName,

		SessionTTL:    seconds( file.Auth.SessionTTLSeconds ),
		LoginTokenTTL: seconds( file.Auth.LoginTokenTTLSeconds ),

		APIKeysEnabled: file.Auth.APIKeys,
		APIKeyTTL:      seconds( file.Auth.APIKeyTTLSeconds ),

		SecretKey:     strings.TrimSpace( file.Server.SecretKey ),
		EncryptAtRest: file.Server.EncryptAtRest,

		RateLimitMax:    file.Server.RateLimitMax,
		RateLimitWindow: seconds( file.Server.RateLimitWindowSeconds ),
		BodyLimit:       file.Server.BodyLimitBytes,

		TrustProxy:     file.Server.TrustProxy,
		TrustedProxies: file.Server.TrustedProxies,
		ProxyHeader:    file.Server.ProxyHeader,
	}
	return
}

func seconds( value int ) ( result time.Duration ) {
	result = time.Duration( value ) * time.Second
	return
}

func ( cfg *Config ) ListenAddress() ( result string ) {
	result = fmt.Sprintf( "%s:%d" , cfg.Host , cfg.Port )
	return
}
