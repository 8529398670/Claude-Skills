// Every call to the server goes through here, so the things that are easy to
// forget -- sending the cookie, attaching the CSRF token, turning a non-2xx
// response into a real Error -- happen once instead of at each call site.
const Api = {
  csrfToken: null,

  async request( path , options ) {
    const settings = options || {};
    let response;
    try {
      response = await fetch( path , {
        method: settings.method || "GET",
        // The session cookie is the only credential; without this an
        // authenticated request silently arrives as an anonymous one.
        credentials: "same-origin",
        headers: settings.headers || { "Content-Type": "application/json" },
        body: settings.raw || ( settings.body ? JSON.stringify( settings.body ) : undefined ),
      } );
    } catch ( networkError ) {
      // >>> feature:logs
      // Never reached the server, so only the browser can log it. Its own
      // words for this ("Failed to fetch") mean nothing to anyone reading them.
      Track.failed( path , 0 );
      // <<< feature:logs
      throw new Error( I18n.get( "errors.network" ) || networkError.message );
    }

    let payload = {};
    try {
      payload = await response.json();
    } catch ( parseError ) {
      payload = {};
    }

    if ( !response.ok ) {
      if ( response.status >= 500 ) Track.failed( path , response.status ); // feature:logs
      // 401 only ever means "no session": a wrong password is a 400 and a
      // wrong code a 403. Api.me() reports it instead, for the page to decide.
      if ( response.status === 401 && path !== "/api/me" && App.currentUser ) this.sessionEnded();
      const error = new Error( this.failureText( payload , response ) );
      error.status = response.status;
      // The whole body, for callers that need more than the code (the
      // Settings screen's problems list, a 409's current version).
      error.payload = payload;
      throw error;
    }
    return payload;
  },

  // The session is gone (it expired, or was signed out from another device), so
  // nothing on this page can work. Say so, rather than leaving every button to
  // fail one at a time.
  sessionEnded() {
    window.location.replace( "/?login=ended" );
  },

  // A failure on the server's side comes with a reference, which is the one
  // thing that lets whoever reads it out be matched to a line in the log.
  failureText( payload , response ) {
    const text = payload.error || response.statusText;
    if ( !payload.request_id ) return text;
    const worded = I18n.get( "errors.reference" );
    return worded ? worded.replace( "{reference}" , payload.request_id ) : text;
  },

  // Any state-changing call carries the per-session CSRF token in the body.
  // Wrapping it here means a new endpoint cannot forget it.
  async post( path , body ) {
    return this.request( path , {
      method: "POST",
      body: Object.assign( {} , body || {} , { csrf_token: this.csrfToken } ),
    } );
  },

  async me() {
    try {
      const payload = await this.request( "/api/me" );
      this.csrfToken = payload.csrf_token;
      return payload;
    } catch ( error ) {
      if ( error.status === 401 ) return { authenticated: false };
      throw error;
    }
  },

  // The one POST with no CSRF token: there is no session yet to hold one.
  // The server guards it by requiring this JSON body instead -- see the
  // login route on the server side.
  login( credential ) {
    return this.request( "/api/login" , { method: "POST" , body: { credential: credential } } );
  },

  // >>> feature:passwords
  // What the signed-out page needs: whether passwords are on, and the salt.
  loginOptions() { return this.request( "/api/login/options" ); },

  // <<< feature:passwords
  // >>> feature:passwords|two_factor
  // Signing in by password, and giving the code an account with two-factor
  // on is asked for. No session yet, so the same rule as login().
  // >>> feature:passwords
  passwordLogin( username , digest ) {
    return this.request( "/api/login/password" , { method: "POST" , body: { username: username , password_digest: digest } } );
  },
  // <<< feature:passwords
  // >>> feature:two_factor
  finishSignIn( challenge , code ) {
    return this.request( "/api/login/two-factor" , { method: "POST" , body: { challenge: challenge , code: code } } );
  },
  // <<< feature:two_factor

  // <<< feature:passwords|two_factor
  // >>> feature:invite_links
  // Invite links: no session yet either, so the same JSON-body rule as
  // login() applies. The credential always goes in the body.
  joinInfo( credential )                { return this.request( "/api/join/info" , { method: "POST" , body: { credential: credential } } ); },
  joinMatch( credential , name )        { return this.request( "/api/join/match" , { method: "POST" , body: { credential: credential , display_name: name } } ); },
  joinCreate( credential , name )       { return this.request( "/api/join/create" , { method: "POST" , body: { credential: credential , display_name: name } } ); },
  joinClaim( credential , userId , pin ) {
    return this.request( "/api/join/claim" , { method: "POST" , body: { credential: credential , user_id: userId , pin: pin || "" } } );
  },

  // <<< feature:invite_links
  // expect is the name the caller believes is stored. The server refuses
  // with a 409 when it is not, so an undo never overwrites a rename made
  // somewhere else in the meantime.
  rename( displayName , expect ) {
    const body = { display_name: displayName };
    if ( expect !== undefined ) body.expect = expect;
    return this.post( "/api/account/rename" , body );
  },
  setPin( pin )                { return this.post( "/api/account/pin" , { pin: pin } ); }, // feature:invite_links
  // The whole set of changed keys; {} puts them all back. See shortcuts.js. // feature:shortcuts
  setShortcuts( shortcuts )    { return this.post( "/api/account/shortcuts" , { shortcuts: shortcuts } ); }, // feature:shortcuts
  // >>> feature:appearance
  // Your whole list of themes (null: back to the defaults) and the one you
  // use; or only the one you use. See theme.js.
  setThemes( themes , id )     { return this.post( "/api/account/themes" , { themes: themes , theme_id: id } ); },
  setTheme( id )               { return this.post( "/api/account/theme" , { theme_id: id } ); },
  // The themes everyone without their own list sees; null is the built-in ones.
  themeDefaults()              { return this.request( "/api/admin/theme-defaults" ); },
  setThemeDefaults( themes )   { return this.post( "/api/admin/theme-defaults" , { themes: themes } ); },
  // <<< feature:appearance
  // >>> feature:passwords
  // digest is what password.js derived; the password itself is never sent.
  // An empty digest removes the password.
  setPassword( username , digest , code ) {
    return this.post( "/api/account/password" , { username: username , password_digest: digest , code: code } );
  },
  // <<< feature:passwords
  // >>> feature:two_factor
  startTwoFactor()             { return this.post( "/api/account/two-factor/start" ); },
  confirmTwoFactor( code )     { return this.post( "/api/account/two-factor/confirm" , { code: code } ); },
  disableTwoFactor( code )     { return this.post( "/api/account/two-factor/disable" , { code: code } ); },
  // <<< feature:two_factor
  // options is LoginLinks.body: how often and how long the link works.
  createMyLoginLink( options ) { return this.post( "/api/account/login-link" , options ); },
  cancelMyLoginLink()          { return this.post( "/api/account/login-link/cancel" ); },
  logout()                     { return this.post( "/api/logout" ); },
  listTeam( offset )           { return this.request( "/api/team" + this.query( { offset: offset } ) ); },
  // params is what Pager.params answers: the table's sort and search, or nothing.
  listUsers( offset , params ) { return this.request( "/api/admin/users" + this.query( { offset: offset , ...params } ) ); },
  // >>> feature:sessions
  userSessions( userId , offset ) { return this.request( "/api/admin/users/" + userId + "/sessions" + this.query( { offset: offset } ) ); },
  revokeUserSession( userId , sessionId ) {
    return this.post( "/api/admin/users/" + userId + "/sessions/" + encodeURIComponent( sessionId ) + "/revoke" );
  },
  // <<< feature:sessions
  createUser( name , role , options ) { return this.post( "/api/admin/users" , Object.assign( { display_name: name , role: role } , options ) ); },
  renameUser( userId , name , expect ) { return this.post( "/api/admin/users/" + userId + "/rename" , { display_name: name , expect: expect } ); },
  setUserRole( userId , role ) { return this.post( "/api/admin/users/" + userId + "/role" , { role: role } ); },
  reissueLogin( userId , options ) { return this.post( "/api/admin/users/" + userId + "/reissue-login" , options ); },
  cancelLoginLink( userId )    { return this.post( "/api/admin/users/" + userId + "/cancel-login-link" ); },
  setDisabled( userId , flag ) { return this.post( "/api/admin/users/" + userId + "/disabled" , { disabled: flag } ); },
  revokeAccess( userId )       { return this.post( "/api/admin/users/" + userId + "/revoke-access" ); },
  deleteUser( userId )         { return this.post( "/api/admin/users/" + userId + "/delete" ); },
  resetTwoFactor( userId )     { return this.post( "/api/admin/users/" + userId + "/reset-two-factor" ); }, // feature:two_factor
  // >>> feature:passwords
  setPasswordAccess( userId , allowed ) {
    return this.post( "/api/owner/users/" + userId + "/password-access" , { allowed: allowed } );
  },
  // <<< feature:passwords
  // >>> feature:invite_links
  listInvites( offset , params ) { return this.request( "/api/admin/invite-links" + this.query( { offset: offset , ...params } ) ); },
  createInvite( label , maxUses , days ) {
    return this.post( "/api/admin/invite-links" , { label: label , max_uses: maxUses , days: days } );
  },
  renameInvite( linkId , label , expect ) {
    return this.post( "/api/admin/invite-links/" + encodeURIComponent( linkId ) + "/rename" , { label: label , expect: expect } );
  },
  // days left out keeps the expiry.
  setInviteLimits( linkId , maxUses , days ) {
    return this.post( "/api/admin/invite-links/" + encodeURIComponent( linkId ) + "/limits" , days === null ? { max_uses: maxUses } : { max_uses: maxUses , days: days } );
  },
  switchOnInvite( linkId )     { return this.post( "/api/admin/invite-links/" + encodeURIComponent( linkId ) + "/switch-on" ); },
  revokeInvite( linkId )       { return this.post( "/api/admin/invite-links/" + encodeURIComponent( linkId ) + "/revoke" ); },
  deleteInvite( linkId )       { return this.post( "/api/admin/invite-links/" + encodeURIComponent( linkId ) + "/delete" ); },
  restoreInvite( linkId )      { return this.post( "/api/admin/invite-links/" + encodeURIComponent( linkId ) + "/restore" ); },
  // <<< feature:invite_links

  // >>> feature:api_keys
  // API keys. createKey's expiresInDays is passed through as-is including null,
  // because the server reads absent as "use the default" and 0 as "never
  // expires" -- collapsing them here would lose the distinction.
  listKeys( offset , params )  { return this.request( "/api/keys" + this.query( { offset: offset , ...params } ) ); },
  createKey( name , role , expiresInDays ) {
    return this.post( "/api/keys" , { name: name , role: role , expires_in_days: expiresInDays } );
  },
  // expect is the name it is replacing: see Undo in the frontend reference.
  renameKey( keyId , name , expect ) {
    return this.post( "/api/keys/" + encodeURIComponent( keyId ) + "/rename" , { name: name , expect: expect } );
  },
  setKeyExpiry( keyId , days ) {
    return this.post( "/api/keys/" + encodeURIComponent( keyId ) + "/expiry" , { expires_in_days: days } );
  },
  revokeKey( keyId )           { return this.post( "/api/keys/" + encodeURIComponent( keyId ) + "/revoke" ); },
  deleteKey( keyId )           { return this.post( "/api/keys/" + encodeURIComponent( keyId ) + "/delete" ); },
  restoreKey( keyId )          { return this.post( "/api/keys/" + encodeURIComponent( keyId ) + "/restore" ); },
  listAllKeys( offset , params ) { return this.request( "/api/admin/keys" + this.query( { offset: offset , ...params } ) ); },
  createKeyFor( userId , name , role , expiresInDays ) {
    return this.post( "/api/admin/keys" , { user_id: userId , name: name , role: role , expires_in_days: expiresInDays } );
  },
  renameAnyKey( keyId , name , expect ) {
    return this.post( "/api/admin/keys/" + encodeURIComponent( keyId ) + "/rename" , { name: name , expect: expect } );
  },
  setAnyKeyExpiry( keyId , days ) {
    return this.post( "/api/admin/keys/" + encodeURIComponent( keyId ) + "/expiry" , { expires_in_days: days } );
  },
  revokeAnyKey( keyId )        { return this.post( "/api/admin/keys/" + encodeURIComponent( keyId ) + "/revoke" ); },
  deleteAnyKey( keyId )        { return this.post( "/api/admin/keys/" + encodeURIComponent( keyId ) + "/delete" ); },
  restoreAnyKey( keyId )       { return this.post( "/api/admin/keys/" + encodeURIComponent( keyId ) + "/restore" ); },

  // <<< feature:api_keys
  // >>> feature:logs
  // The activity log (owner only; the server answers anyone else with 404).
  // q is the search box as typed; the server parses it.
  logDays( query ) {
    return this.request( "/api/owner/logs/days" + this.query( { q: query } ) );
  },
  logDay( date , query , before ) {
    return this.request( "/api/owner/logs/days/" + encodeURIComponent( date ) + this.query( { q: query , before: before } ) );
  },
  logStreamPath()              { return "/api/owner/logs/stream"; },
  logDownloadPath( date )      { return "/api/owner/logs/days/" + encodeURIComponent( date ) + "/download"; },
  deleteLogDay( date )         { return this.post( "/api/owner/logs/days/" + encodeURIComponent( date ) + "/delete" ); },
  pruneLogs()                  { return this.post( "/api/owner/logs/prune" ); },
  ipNames()                    { return this.request( "/api/owner/logs/ip-names" ); },
  setIpName( target , name )   { return this.post( "/api/owner/logs/ip-names" , { target: target , name: name } ); },
  deleteIpName( target )       { return this.post( "/api/owner/logs/ip-names/delete" , { target: target } ); },

  // <<< feature:logs
  // >>> feature:files
  // Files. A piece's body is the file's own bytes, so unlike every other POST
  // its CSRF token goes in a header.
  files( offset , params )     { return this.request( "/api/files" + this.query( { offset: offset , ...params } ) ); },
  allFiles( offset , params )  { return this.request( "/api/admin/files" + this.query( { offset: offset , ...params } ) ); },
  storage()                    { return this.request( "/api/account/storage" ); },
  allStorage()                 { return this.request( "/api/admin/storage" ); },
  createFile( name , size , type ) { return this.post( "/api/files" , { name: name , size: size , type: type } ); },
  sendFileChunk( id , index , piece ) {
    return this.request( "/api/files/" + encodeURIComponent( id ) + "/chunks/" + index , {
      method: "POST",
      headers: { "Content-Type": "application/octet-stream" , "X-CSRF-Token": this.csrfToken },
      raw: piece,
    } );
  },
  finishFile( id )             { return this.post( "/api/files/" + encodeURIComponent( id ) + "/finish" ); },
  renameFile( id , name , expect ) {
    return this.post( "/api/files/" + encodeURIComponent( id ) + "/rename" , { name: name , expect: expect } );
  },
  renameAnyFile( id , name , expect ) {
    return this.post( "/api/admin/files/" + encodeURIComponent( id ) + "/rename" , { name: name , expect: expect } );
  },
  deleteFile( id )             { return this.post( "/api/files/" + encodeURIComponent( id ) + "/delete" ); },
  restoreFile( id )            { return this.post( "/api/files/" + encodeURIComponent( id ) + "/restore" ); },
  deleteAnyFile( id )          { return this.post( "/api/admin/files/" + encodeURIComponent( id ) + "/delete" ); },
  restoreAnyFile( id )         { return this.post( "/api/admin/files/" + encodeURIComponent( id ) + "/restore" ); },
  fileDownloadPath( id )       { return "/api/files/" + encodeURIComponent( id ) + "/download"; },
  anyFileDownloadPath( id )    { return "/api/admin/files/" + encodeURIComponent( id ) + "/download"; },

  // <<< feature:files
  // >>> feature:webhooks
  // Webhooks (owner only). A test is queued, not sent, by the time this answers.
  webhooks( offset , params )  { return this.request( "/api/owner/webhooks" + this.query( { offset: offset , ...params } ) ); },
  webhookEvents()              { return this.request( "/api/owner/webhooks/events" ); },
  createWebhook( name , url , events ) {
    return this.post( "/api/owner/webhooks" , { name: name , url: url , events: events } );
  },
  setWebhookEnabled( id , flag ) { return this.post( "/api/owner/webhooks/" + encodeURIComponent( id ) + "/enabled" , { enabled: flag } ); },
  renameWebhook( id , name , expect ) {
    return this.post( "/api/owner/webhooks/" + encodeURIComponent( id ) + "/rename" , { name: name , expect: expect } );
  },
  // target and expect are each { url , events }.
  setWebhookTarget( id , target , expect ) {
    return this.post( "/api/owner/webhooks/" + encodeURIComponent( id ) + "/target" , { url: target.url , events: target.events , expect: expect } );
  },
  rotateWebhookSecret( id )    { return this.post( "/api/owner/webhooks/" + encodeURIComponent( id ) + "/rotate-secret" ); },
  deleteWebhook( id )          { return this.post( "/api/owner/webhooks/" + encodeURIComponent( id ) + "/delete" ); },
  restoreWebhook( id )         { return this.post( "/api/owner/webhooks/" + encodeURIComponent( id ) + "/restore" ); },
  testWebhook( id )            { return this.post( "/api/owner/webhooks/" + encodeURIComponent( id ) + "/test" ); },
  webhookDeliveries( id , offset ) {
    return this.request( "/api/owner/webhooks/" + encodeURIComponent( id ) + "/deliveries" + this.query( { offset: offset } ) );
  },

  resendDelivery( id , deliveryId ) {
    return this.post( "/api/owner/webhooks/" + encodeURIComponent( id ) + "/deliveries/" + encodeURIComponent( deliveryId ) + "/resend" );
  },

  // <<< feature:webhooks
  // >>> feature:jobs
  // Background jobs (owner only). A run outlives the call that starts it.
  jobs()                       { return this.request( "/api/owner/jobs" ); },
  runJob( name )               { return this.post( "/api/owner/jobs/" + encodeURIComponent( name ) + "/run" ); },

  // <<< feature:jobs
  // >>> feature:sessions
  // Where this account is signed in.
  sessions( offset )           { return this.request( "/api/account/sessions" + this.query( { offset: offset } ) ); },
  labelSession( sessionId , label , expect ) {
    return this.post( "/api/account/sessions/" + encodeURIComponent( sessionId ) + "/label" , { label: label , expect: expect } );
  },
  revokeSession( sessionId )   { return this.post( "/api/account/sessions/" + encodeURIComponent( sessionId ) + "/revoke" ); },
  revokeOtherSessions()        { return this.post( "/api/account/sessions/revoke-others" ); },

  // <<< feature:sessions
  // >>> feature:settings
  // config.yaml as the Settings screen (owner only). version is the one
  // settings() returned; the server refuses a save against any other.
  settings()                   { return this.request( "/api/owner/settings" ); },
  saveSettings( version , changes ) {
    return this.post( "/api/owner/settings" , { version: version , changes: changes } );
  },

  // <<< feature:settings
  // "?a=1&b=2" from the fields that have a value, or "" when none do.
  query( fields ) {
    const params = new URLSearchParams();
    Object.entries( fields ).forEach( function ( entry ) {
      if ( entry[ 1 ] !== undefined && entry[ 1 ] !== null && entry[ 1 ] !== "" ) params.set( entry[ 0 ] , String( entry[ 1 ] ) );
    } );
    const text = params.toString();
    return text ? "?" + text : "";
  },
};
