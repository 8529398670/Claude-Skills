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
        headers: { "Content-Type": "application/json" },
        body: settings.body ? JSON.stringify( settings.body ) : undefined,
      } );
    } catch ( networkError ) {
      // Never reached the server, so only the browser can log it.
      Track.failed( path , 0 );
      throw networkError;
    }

    let payload = {};
    try {
      payload = await response.json();
    } catch ( parseError ) {
      payload = {};
    }

    if ( !response.ok ) {
      if ( response.status >= 500 ) Track.failed( path , response.status );
      const error = new Error( payload.error || response.statusText );
      error.status = response.status;
      // The whole body, for callers that need more than the code (the
      // Settings screen's problems list, a 409's current version).
      error.payload = payload;
      throw error;
    }
    return payload;
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

  // What the signed-out page needs: whether passwords are on, and the salt.
  loginOptions() { return this.request( "/api/login/options" ); },

  // Signing in by password, and giving the code an account with two-factor
  // on is asked for. No session yet, so the same rule as login().
  passwordLogin( username , digest ) {
    return this.request( "/api/login/password" , { method: "POST" , body: { username: username , password_digest: digest } } );
  },
  finishSignIn( challenge , code ) {
    return this.request( "/api/login/two-factor" , { method: "POST" , body: { challenge: challenge , code: code } } );
  },

  // Invite links: no session yet either, so the same JSON-body rule as
  // login() applies. The credential always goes in the body.
  joinInfo( credential )                { return this.request( "/api/join/info" , { method: "POST" , body: { credential: credential } } ); },
  joinMatch( credential , name )        { return this.request( "/api/join/match" , { method: "POST" , body: { credential: credential , display_name: name } } ); },
  joinCreate( credential , name )       { return this.request( "/api/join/create" , { method: "POST" , body: { credential: credential , display_name: name } } ); },
  joinClaim( credential , userId , pin ) {
    return this.request( "/api/join/claim" , { method: "POST" , body: { credential: credential , user_id: userId , pin: pin || "" } } );
  },

  // expect is the name the caller believes is stored. The server refuses
  // with a 409 when it is not, so an undo never overwrites a rename made
  // somewhere else in the meantime.
  rename( displayName , expect ) {
    const body = { display_name: displayName };
    if ( expect !== undefined ) body.expect = expect;
    return this.post( "/api/account/rename" , body );
  },
  setPin( pin )                { return this.post( "/api/account/pin" , { pin: pin } ); },
  // digest is what password.js derived; the password itself is never sent.
  // An empty digest removes the password.
  setPassword( username , digest , code ) {
    return this.post( "/api/account/password" , { username: username , password_digest: digest , code: code } );
  },
  startTwoFactor()             { return this.post( "/api/account/two-factor/start" ); },
  confirmTwoFactor( code )     { return this.post( "/api/account/two-factor/confirm" , { code: code } ); },
  disableTwoFactor( code )     { return this.post( "/api/account/two-factor/disable" , { code: code } ); },
  createMyLoginLink()          { return this.post( "/api/account/login-link" ); },
  logout()                     { return this.post( "/api/logout" ); },
  listTeam()                   { return this.request( "/api/team" ); },
  listUsers()                  { return this.request( "/api/admin/users" ); },
  createUser( name , role )    { return this.post( "/api/admin/users" , { display_name: name , role: role } ); },
  reissueLogin( userId )       { return this.post( "/api/admin/users/" + userId + "/reissue-login" ); },
  setDisabled( userId , flag ) { return this.post( "/api/admin/users/" + userId + "/disabled" , { disabled: flag } ); },
  revokeAccess( userId )       { return this.post( "/api/admin/users/" + userId + "/revoke-access" ); },
  deleteUser( userId )         { return this.post( "/api/admin/users/" + userId + "/delete" ); },
  resetTwoFactor( userId )     { return this.post( "/api/admin/users/" + userId + "/reset-two-factor" ); },
  setPasswordAccess( userId , allowed ) {
    return this.post( "/api/owner/users/" + userId + "/password-access" , { allowed: allowed } );
  },
  listInvites()                { return this.request( "/api/admin/invite-links" ); },
  createInvite( label , maxUses , days ) {
    return this.post( "/api/admin/invite-links" , { label: label , max_uses: maxUses , days: days } );
  },
  revokeInvite( linkId )       { return this.post( "/api/admin/invite-links/" + encodeURIComponent( linkId ) + "/revoke" ); },
  deleteInvite( linkId )       { return this.post( "/api/admin/invite-links/" + encodeURIComponent( linkId ) + "/delete" ); },
  restoreInvite( linkId )      { return this.post( "/api/admin/invite-links/" + encodeURIComponent( linkId ) + "/restore" ); },

  // API keys. createKey's expiresInDays is passed through as-is including null,
  // because the server reads absent as "use the default" and 0 as "never
  // expires" -- collapsing them here would lose the distinction.
  listKeys()                   { return this.request( "/api/keys" ); },
  createKey( name , role , expiresInDays ) {
    return this.post( "/api/keys" , { name: name , role: role , expires_in_days: expiresInDays } );
  },
  revokeKey( keyId )           { return this.post( "/api/keys/" + encodeURIComponent( keyId ) + "/revoke" ); },
  deleteKey( keyId )           { return this.post( "/api/keys/" + encodeURIComponent( keyId ) + "/delete" ); },
  restoreKey( keyId )          { return this.post( "/api/keys/" + encodeURIComponent( keyId ) + "/restore" ); },
  listAllKeys()                { return this.request( "/api/admin/keys" ); },
  revokeAnyKey( keyId )        { return this.post( "/api/admin/keys/" + encodeURIComponent( keyId ) + "/revoke" ); },
  deleteAnyKey( keyId )        { return this.post( "/api/admin/keys/" + encodeURIComponent( keyId ) + "/delete" ); },
  restoreAnyKey( keyId )       { return this.post( "/api/admin/keys/" + encodeURIComponent( keyId ) + "/restore" ); },

  // The activity log (owner only; the server answers anyone else with 404).
  // q is the search box as typed; the server parses it.
  logDays( query ) {
    return this.request( "/api/owner/logs/days" + this.query( { q: query } ) );
  },
  logDay( date , query , before ) {
    return this.request( "/api/owner/logs/days/" + encodeURIComponent( date ) + this.query( { q: query , before: before } ) );
  },
  logDownloadPath( date )      { return "/api/owner/logs/days/" + encodeURIComponent( date ) + "/download"; },
  deleteLogDay( date )         { return this.post( "/api/owner/logs/days/" + encodeURIComponent( date ) + "/delete" ); },
  pruneLogs()                  { return this.post( "/api/owner/logs/prune" ); },

  // config.yaml as the Settings screen (owner only). version is the one
  // settings() returned; the server refuses a save against any other.
  settings()                   { return this.request( "/api/owner/settings" ); },
  saveSettings( version , changes ) {
    return this.post( "/api/owner/settings" , { version: version , changes: changes } );
  },

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
