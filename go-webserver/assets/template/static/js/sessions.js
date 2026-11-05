// "Where you're signed in", on the Account screen: every device with a live
// session on this account, and a way to sign one out from here. app.js calls
// Sessions.init() once it knows who is signed in.
//
// Signing a device out is never undoable: it is the button for a lost phone,
// and an undo would let the session back in. So it asks first, like revoking an
// API key. This device has no button here; that is what Log out is for.
const Sessions = {
  async init() {
    Dom.get( "sessions-revoke-others" ).addEventListener( "click" , () => this.onRevokeOthers() );
    window.addEventListener( "hashchange" , () => {
      if ( Nav.current() === "account" ) this.refresh();
    } );
    await this.refresh();
  },

  // A short name out of a User-Agent header. It only has to help someone tell
  // their own devices apart; the order matters, since Edge and Chrome both
  // say Safari, and Android says Linux.
  browser( agent ) {
    const known = [ [ /Edg\// , "Edge" ] , [ /OPR\// , "Opera" ] , [ /Firefox\// , "Firefox" ] , [ /Chrome\// , "Chrome" ] , [ /Safari\// , "Safari" ] ];
    const match = known.find( function ( entry ) { return entry[ 0 ].test( agent ); } );
    return match ? match[ 1 ] : "";
  },

  system( agent ) {
    const known = [ [ /Android/ , "Android" ] , [ /iPhone|iPad/ , "iOS" ] , [ /Mac OS X/ , "macOS" ] , [ /Windows/ , "Windows" ] , [ /Linux/ , "Linux" ] ];
    const match = known.find( function ( entry ) { return entry[ 0 ].test( agent ); } );
    return match ? match[ 1 ] : "";
  },

  device( session ) {
    const agent = session.user_agent || "";
    const browser = this.browser( agent );
    const system = this.system( agent );
    if ( browser && system ) {
      return I18n.get( "sessions.device_name" ).replace( "{browser}" , browser ).replace( "{system}" , system );
    }
    return browser || system || agent || I18n.get( "sessions.unknown_device" );
  },

  when( value ) {
    const parsed = new Date( value || "" );
    return isNaN( parsed.getTime() ) ? I18n.get( "sessions.unknown" ) : parsed.toLocaleString();
  },

  revokeButton( session ) {
    return Dom.el( "button" , {
      class: "btn btn-sm btn-outline-danger text-nowrap",
      text: I18n.get( "sessions.revoke_button" ),
      on: { click: async () => {
        // An empty confirm message means "do not ask", following the same
        // language.yaml rule as every other string here.
        const question = I18n.get( "sessions.revoke_confirm" );
        if ( question !== "" && !window.confirm( question ) ) return;
        try {
          await Api.revokeSession( session.id );
          await this.refresh();
        } catch ( error ) { App.showError( error.message ); }
      } },
    } );
  },

  async onRevokeOthers() {
    const question = I18n.get( "sessions.revoke_others_confirm" );
    if ( question !== "" && !window.confirm( question ) ) return;
    try {
      await Api.revokeOtherSessions();
      await this.refresh();
    } catch ( error ) { App.showError( error.message ); }
  },

  async refresh() {
    let sessions;
    try {
      sessions = await Api.sessions();
    } catch ( error ) {
      App.showError( error.message );
      return;
    }
    const body = Dom.get( "sessions-table" ).querySelector( "tbody" );
    Dom.clear( body );
    sessions.forEach( ( session ) => {
      const name = [ Dom.el( "span" , { text: this.device( session ) } ) ];
      if ( session.current ) name.push( Dom.el( "span" , { class: "ms-2" , children: [ Dom.badge( I18n.get( "sessions.this_device" ) , "success" ) ] } ) );
      body.appendChild( Dom.el( "tr" , { children: [
        Dom.el( "td" , { children: name } ),
        Dom.el( "td" , { text: session.ip || I18n.get( "sessions.unknown" ) } ),
        Dom.el( "td" , { text: this.when( session.last_seen_at ) } ),
        Dom.el( "td" , { text: this.when( session.created_at ) } ),
        Dom.el( "td" , { children: session.current ? [] : [ this.revokeButton( session ) ] } ),
      ] } ) );
    } );
    Dom.show( Dom.get( "sessions-revoke-others" ) , sessions.length > 1 );
  },
};
