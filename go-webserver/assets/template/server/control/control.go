// Package control is the one door into the database for every process that is
// not the web server.
//
// bolt takes an exclusive lock on its file, so exactly one process can have
// the database open at a time. Rather than working around that, this package
// leans on it: the server process owns the database, and anything else that
// needs to act on it -- the manage CLI, a cron job, a sidecar -- asks the
// server to do so over a unix socket in the app directory.
//
// That is the cheapest answer available in security terms. There is no second
// process holding the secret key, no second copy of the model layer's rules
// to keep in step, and no new network surface: the socket is a file, and it
// is reachable only by something that can already read the app directory.
//
// # Why there is no token on this socket
//
// There is deliberately no password, token, or handshake here. The socket
// lives in AppDir, which is mode 0700, next to secret.key and app.db. Any
// process that can open the socket can already read the key and the database
// directly, so a credential would guard nothing while adding one more secret
// to store and rotate. The filesystem permission is the authentication.
//
// This is also why the socket must never be moved somewhere world-readable,
// and why the server refuses to bind it over a TCP address: the whole
// argument depends on the socket being as hard to reach as the database file
// sitting beside it.
package control

import (
	context "context"
	json "encoding/json"
	errors "errors"
	fmt "fmt"
	log "log"
	net "net"
	http "net/http"
	os "os"
	runtime "runtime"
	strconv "strconv"
	time "time"

	config "{{MODULE_PATH}}/server/config"
	db "{{MODULE_PATH}}/server/db"
	models "{{MODULE_PATH}}/server/models"
)

// maxRequestBytes caps a control request body. Nothing this API accepts is
// large, and a local caller is trusted, but a bounded reader costs nothing and
// keeps a malformed client from being able to exhaust memory.
const maxRequestBytes = 64 * 1024

// maxSocketPathLength is the kernel's limit on a unix socket path: the
// sun_path field is 104 bytes on macOS and the BSDs, 108 on Linux, including
// the terminator. 103 is the safe number everywhere.
//
// This is worth checking by hand because the failure is otherwise a bare
// "bind: invalid argument" from the kernel, which names neither the limit nor
// the path that broke it. A deep APP_DIR is all it takes.
const maxSocketPathLength = 103

// Server is the control listener. It is created after the database is open,
// so its existence is itself the signal that a live server holds the lock.
type Server struct {
	store    *db.Store
	cfg      *config.Config
	http     *http.Server
	listener net.Listener
	path     string
}

// Listen binds the control socket and starts serving in the background.
//
// A socket file left behind by a crashed process is removed first. That is
// safe precisely because bolt already granted us the database lock: if this
// process holds the lock, no live server is using that stale socket.
func Listen( store *db.Store , cfg *config.Config ) ( server *Server , err error ) {
	if cfg.ControlSocketEnabled == false {
		return
	}

	if len( cfg.ControlSocketPath ) > maxSocketPathLength {
		err = fmt.Errorf(
			"control socket path is %d characters, past the %d the kernel allows: %s\n"+
				"Set APP_DIR to a shorter path, or set CONTROL_SOCKET=false to run without the socket -- manage will then need the server stopped." ,
			len( cfg.ControlSocketPath ) , maxSocketPathLength , cfg.ControlSocketPath )
		return
	}

	if _ , stat_err := os.Stat( cfg.ControlSocketPath ); stat_err == nil {
		if remove_err := os.Remove( cfg.ControlSocketPath ); remove_err != nil {
			err = fmt.Errorf( "could not clear stale control socket %s: %w" , cfg.ControlSocketPath , remove_err )
			return
		}
	}

	listener , err := net.Listen( "unix" , cfg.ControlSocketPath )
	if err != nil {
		err = fmt.Errorf( "could not bind control socket %s: %w" , cfg.ControlSocketPath , err )
		return
	}

	// Belt and braces on top of the 0700 app directory. On Windows this is a
	// no-op and the directory ACL is what protects the socket -- see the
	// package docs; the security argument is the same either way, but it is
	// worth knowing which mechanism is doing the work.
	if runtime.GOOS != "windows" {
		if chmod_err := os.Chmod( cfg.ControlSocketPath , 0o600 ); chmod_err != nil {
			listener.Close()
			os.Remove( cfg.ControlSocketPath )
			err = fmt.Errorf( "could not restrict control socket permissions: %w" , chmod_err )
			return
		}
	}

	server = &Server{
		store:    store,
		cfg:      cfg,
		listener: listener,
		path:     cfg.ControlSocketPath,
	}
	server.http = &http.Server{
		Handler: server.routes(),
		// A local peer should never be slow, so a short header timeout costs
		// nothing here and keeps a wedged client from holding a connection.
		ReadHeaderTimeout: 5 * time.Second,
	}

	go func() {
		serve_err := server.http.Serve( listener )
		if serve_err != nil && errors.Is( serve_err , http.ErrServerClosed ) == false {
			log.Printf( "[control] listener stopped: %v" , serve_err )
		}
	}()
	return
}

// Close stops serving and unlinks the socket, so a later start does not have
// to reason about whether a leftover file means a live server.
func ( server *Server ) Close() ( err error ) {
	if server == nil {
		return
	}
	ctx , cancel := context.WithTimeout( context.Background() , 5*time.Second )
	defer cancel()
	server.http.Shutdown( ctx )
	os.Remove( server.path )
	return
}

// Path reports where the socket was bound, for the startup log.
func ( server *Server ) Path() ( result string ) {
	if server == nil {
		result = "disabled"
		return
	}
	result = server.path
	return
}

func ( server *Server ) routes() ( handler http.Handler ) {
	mux := http.NewServeMux()
	mux.HandleFunc( "GET /v1/health" , server.handleHealth )
	mux.HandleFunc( "GET /v1/users" , server.handleListUsers )
	mux.HandleFunc( "POST /v1/users" , server.handleCreateUser )
	mux.HandleFunc( "POST /v1/users/{id}/login-token" , server.handleReissueLogin )
	mux.HandleFunc( "POST /v1/users/{id}/disabled" , server.handleSetDisabled )
	mux.HandleFunc( "GET /v1/api-keys" , server.handleListAPIKeys )
	mux.HandleFunc( "POST /v1/api-keys" , server.handleCreateAPIKey )
	mux.HandleFunc( "POST /v1/api-keys/{id}/revoke" , server.handleRevokeAPIKey )
	handler = mux
	return
}

// writeJSON is the single place a control response is produced, so every
// handler answers in the same shape.
func writeJSON( w http.ResponseWriter , status int , payload any ) {
	w.Header().Set( "Content-Type" , "application/json" )
	w.WriteHeader( status )
	json.NewEncoder( w ).Encode( payload )
}

func writeError( w http.ResponseWriter , status int , message string ) {
	writeJSON( w , status , map[ string ]string{ "error": message } )
}

func decodeBody( w http.ResponseWriter , r *http.Request , out any ) ( ok bool ) {
	decoder := json.NewDecoder( http.MaxBytesReader( w , r.Body , maxRequestBytes ) )
	decoder.DisallowUnknownFields()
	if err := decoder.Decode( out ); err != nil {
		writeError( w , http.StatusBadRequest , fmt.Sprintf( "malformed request body: %v" , err ) )
		return
	}
	ok = true
	return
}

func pathUserID( w http.ResponseWriter , r *http.Request ) ( user_id uint64 , ok bool ) {
	user_id , err := strconv.ParseUint( r.PathValue( "id" ) , 10 , 64 )
	if err != nil {
		writeError( w , http.StatusBadRequest , "user id must be a positive integer" )
		return
	}
	ok = true
	return
}

// HealthResponse tells a client it is talking to a live server, and which one.
type HealthResponse struct {
	ProjectName string `json:"project_name"`
	AppDir      string `json:"app_dir"`
}

// UserResponse is a stored user plus the fields a caller would otherwise have
// to recompute. Credential is set only by the two endpoints that mint a login
// link, and is the one moment it is ever visible.
type UserResponse struct {
	ID          uint64 `json:"id"`
	DisplayName string `json:"display_name"`
	Role        string `json:"role"`
	Disabled    bool   `json:"disabled"`
	Credential  string `json:"credential,omitempty"`
}

func userResponse( user *models.User , credential string ) ( result UserResponse ) {
	result = UserResponse{
		ID:          user.ID,
		DisplayName: user.DisplayName,
		Role:        user.Role,
		Disabled:    user.Disabled(),
		Credential:  credential,
	}
	return
}

// CreateUserRequest is the body of POST /v1/users.
type CreateUserRequest struct {
	DisplayName string `json:"display_name"`
	Role        string `json:"role"`
}

// SetDisabledRequest is the body of POST /v1/users/{id}/disabled.
type SetDisabledRequest struct {
	Disabled bool `json:"disabled"`
}

// APIKeyResponse is a stored API key as a caller sees it. Credential is set
// only by the endpoint that mints one, and is the one moment it is ever
// visible; the stored hash is never exposed at all.
type APIKeyResponse struct {
	ID          string     `json:"id"`
	UserID      uint64     `json:"user_id"`
	DisplayName string     `json:"display_name"`
	Name        string     `json:"name"`
	Role        string     `json:"role"`
	Live        bool       `json:"live"`
	Revoked     bool       `json:"revoked"`
	Expired     bool       `json:"expired"`
	CreatedAt   time.Time  `json:"created_at"`
	ExpiresAt   *time.Time `json:"expires_at,omitempty"`
	LastUsedAt  *time.Time `json:"last_used_at,omitempty"`
	RevokedAt   *time.Time `json:"revoked_at,omitempty"`
	Credential  string     `json:"credential,omitempty"`
}

// CreateAPIKeyRequest is the body of POST /v1/api-keys. ExpiresInDays is a
// pointer for the same reason it is in the HTTP API: absent means "the
// server's default", 0 means "never expires", and those must stay
// distinguishable.
type CreateAPIKeyRequest struct {
	UserID        uint64 `json:"user_id"`
	Name          string `json:"name"`
	Role          string `json:"role"`
	ExpiresInDays *int   `json:"expires_in_days"`
}

func apiKeyResponse( key *models.APIKey , display_name string , credential string ) ( result APIKeyResponse ) {
	result = APIKeyResponse{
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

// displayNames builds the id -> name lookup the key listings need, in one
// pass, rather than reading a user record per key.
func ( server *Server ) displayNames() ( names map[ uint64 ]string , err error ) {
	names = map[ uint64 ]string{}
	users , err := models.ListUsers( server.store )
	if err != nil { return }
	for _ , user := range users {
		names[ user.ID ] = user.DisplayName
	}
	return
}

// handleListAPIKeys lists every key, or one user's with ?user_id=N.
func ( server *Server ) handleListAPIKeys( w http.ResponseWriter , r *http.Request ) {
	var user_id uint64
	if raw := r.URL.Query().Get( "user_id" ); raw != "" {
		parsed , parse_err := strconv.ParseUint( raw , 10 , 64 )
		if parse_err != nil {
			writeError( w , http.StatusBadRequest , "user_id must be a positive integer" )
			return
		}
		user_id = parsed
	}

	var keys []*models.APIKey
	var err error
	if user_id == 0 {
		keys , err = models.ListAPIKeys( server.store )
	} else {
		keys , err = models.ListAPIKeysForUser( server.store , user_id )
	}
	if err != nil {
		writeError( w , http.StatusInternalServerError , err.Error() )
		return
	}

	names , err := server.displayNames()
	if err != nil {
		writeError( w , http.StatusInternalServerError , err.Error() )
		return
	}

	out := []APIKeyResponse{}
	for _ , key := range keys {
		out = append( out , apiKeyResponse( key , names[ key.UserID ] , "" ) )
	}
	writeJSON( w , http.StatusOK , out )
}

// handleCreateAPIKey mints a key on someone's behalf -- the CLI's way to hand
// a CI system or a cron job a credential without anyone signing in.
//
// It goes through models.IssueAPIKey like every other path, so the ceiling
// that a key cannot outrank its owner is enforced here too: this endpoint
// cannot mint an admin key for a non-admin account.
func ( server *Server ) handleCreateAPIKey( w http.ResponseWriter , r *http.Request ) {
	if server.cfg.APIKeysEnabled == false {
		writeError( w , http.StatusForbidden , "api keys are disabled on this server (API_KEYS=false)" )
		return
	}

	body := CreateAPIKeyRequest{}
	if decodeBody( w , r , &body ) == false { return }

	if models.ValidAPIKeyName( body.Name ) == false {
		writeError( w , http.StatusBadRequest , "name must be 1-80 characters" )
		return
	}
	role := body.Role
	if role == "" {
		role = models.RoleUser
	}
	if models.ValidRole( role ) == false {
		writeError( w , http.StatusBadRequest , "role must be admin or user" )
		return
	}
	ttl , ok := models.APIKeyTTLFromDays( body.ExpiresInDays , server.cfg.APIKeyTTL )
	if ok == false {
		writeError( w , http.StatusBadRequest , "expires_in_days must be 0 (never) or up to 3650" )
		return
	}

	user , err := models.GetUser( server.store , body.UserID )
	if err != nil {
		writeError( w , http.StatusNotFound , fmt.Sprintf( "no user with id %d" , body.UserID ) )
		return
	}

	credential , key , err := models.IssueAPIKey( server.store , user.ID , body.Name , role , ttl )
	if err != nil {
		writeError( w , http.StatusBadRequest , err.Error() )
		return
	}
	writeJSON( w , http.StatusOK , apiKeyResponse( key , user.DisplayName , credential ) )
}

func ( server *Server ) handleRevokeAPIKey( w http.ResponseWriter , r *http.Request ) {
	key_id := r.PathValue( "id" )
	key , err := models.GetAPIKey( server.store , key_id )
	if err != nil {
		writeError( w , http.StatusNotFound , fmt.Sprintf( "no api key with id %s" , key_id ) )
		return
	}
	if err = models.RevokeAPIKey( server.store , key.ID ); err != nil {
		writeError( w , http.StatusInternalServerError , err.Error() )
		return
	}

	// Re-read rather than patching the copy in hand, so the response reflects
	// what is actually stored.
	key , err = models.GetAPIKey( server.store , key.ID )
	if err != nil {
		writeError( w , http.StatusInternalServerError , err.Error() )
		return
	}
	names , err := server.displayNames()
	if err != nil {
		writeError( w , http.StatusInternalServerError , err.Error() )
		return
	}
	writeJSON( w , http.StatusOK , apiKeyResponse( key , names[ key.UserID ] , "" ) )
}

func ( server *Server ) handleHealth( w http.ResponseWriter , r *http.Request ) {
	writeJSON( w , http.StatusOK , HealthResponse{
		ProjectName: server.cfg.ProjectName,
		AppDir:      server.cfg.AppDir,
	} )
}

func ( server *Server ) handleListUsers( w http.ResponseWriter , r *http.Request ) {
	users , err := models.ListUsers( server.store )
	if err != nil {
		writeError( w , http.StatusInternalServerError , err.Error() )
		return
	}
	out := []UserResponse{}
	for _ , user := range users {
		out = append( out , userResponse( user , "" ) )
	}
	writeJSON( w , http.StatusOK , out )
}

// handleCreateUser validates through the same models helpers the web admin
// routes use, so a user created over the socket cannot be shaped differently
// from one created in the UI.
func ( server *Server ) handleCreateUser( w http.ResponseWriter , r *http.Request ) {
	body := CreateUserRequest{}
	if decodeBody( w , r , &body ) == false { return }

	if models.ValidDisplayName( body.DisplayName ) == false {
		writeError( w , http.StatusBadRequest , "display name must be 1-80 characters" )
		return
	}
	if models.ValidRole( body.Role ) == false {
		writeError( w , http.StatusBadRequest , "role must be admin or user" )
		return
	}

	user , err := models.CreateUser( server.store , body.DisplayName , body.Role )
	if err != nil {
		writeError( w , http.StatusInternalServerError , err.Error() )
		return
	}
	credential , err := models.IssueLoginToken( server.store , user.ID , server.cfg.LoginTokenTTL )
	if err != nil {
		writeError( w , http.StatusInternalServerError , err.Error() )
		return
	}
	writeJSON( w , http.StatusOK , userResponse( user , credential ) )
}

func ( server *Server ) handleReissueLogin( w http.ResponseWriter , r *http.Request ) {
	user_id , ok := pathUserID( w , r )
	if ok == false { return }

	user , err := models.GetUser( server.store , user_id )
	if err != nil {
		writeError( w , http.StatusNotFound , fmt.Sprintf( "no user with id %d" , user_id ) )
		return
	}
	credential , err := models.IssueLoginToken( server.store , user.ID , server.cfg.LoginTokenTTL )
	if err != nil {
		writeError( w , http.StatusInternalServerError , err.Error() )
		return
	}
	writeJSON( w , http.StatusOK , userResponse( user , credential ) )
}

// handleSetDisabled revokes sessions on the same request that disables the
// account, matching the admin route. Doing it here rather than leaving it to
// the caller is what keeps "disabled" meaning "logged out now" no matter
// which door the change came through.
func ( server *Server ) handleSetDisabled( w http.ResponseWriter , r *http.Request ) {
	user_id , ok := pathUserID( w , r )
	if ok == false { return }

	body := SetDisabledRequest{}
	if decodeBody( w , r , &body ) == false { return }

	user , err := models.GetUser( server.store , user_id )
	if err != nil {
		writeError( w , http.StatusNotFound , fmt.Sprintf( "no user with id %d" , user_id ) )
		return
	}
	if err = models.SetUserDisabled( server.store , user.ID , body.Disabled ); err != nil {
		writeError( w , http.StatusInternalServerError , err.Error() )
		return
	}
	if body.Disabled {
		if err = models.DestroyAllSessionsForUser( server.store , user.ID ); err != nil {
			writeError( w , http.StatusInternalServerError , err.Error() )
			return
		}
	}

	// Re-read rather than patching the copy in hand, so the response
	// reflects what is actually stored.
	user , err = models.GetUser( server.store , user.ID )
	if err != nil {
		writeError( w , http.StatusInternalServerError , err.Error() )
		return
	}
	writeJSON( w , http.StatusOK , userResponse( user , "" ) )
}
