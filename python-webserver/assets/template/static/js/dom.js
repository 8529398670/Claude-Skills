// Tiny DOM helpers, shared by every page. Kept in its own file so a new page
// can use them without loading the account/admin logic in app.js.
//
// The reason this exists at all rather than each page reaching for innerHTML:
// building elements through createElement + textContent means a display name
// containing "<script>" is rendered as those literal characters. Assembling
// the same row as an HTML string would execute it. There is no escaping
// helper here on purpose -- the safe path should be the only path.
const Dom = {
  get( id ) {
    return document.getElementById( id );
  },

  all( selector , root ) {
    return Array.from( ( root || document ).querySelectorAll( selector ) );
  },

  show( element , visible ) {
    if ( element ) element.hidden = !visible;
  },

  text( element , value ) {
    if ( element ) element.textContent = value == null ? "" : String( value );
  },

  clear( element ) {
    if ( element ) element.replaceChildren();
  },

  // el( "button", { class: "small", text: name, on: { click: fn } } )
  el( tag , options ) {
    const settings = options || {};
    const node = document.createElement( tag );
    if ( settings.class ) node.className = settings.class;
    if ( settings.text != null ) node.textContent = String( settings.text );
    if ( settings.attrs ) {
      Object.entries( settings.attrs ).forEach( function ( entry ) {
        node.setAttribute( entry[ 0 ] , entry[ 1 ] );
      } );
    }
    if ( settings.on ) {
      Object.entries( settings.on ).forEach( function ( entry ) {
        node.addEventListener( entry[ 0 ] , entry[ 1 ] );
      } );
    }
    ( settings.children || [] ).forEach( function ( child ) {
      node.appendChild( child );
    } );
    return node;
  },

  // Runs work with button showing that it is busy: disabled, so a second click
  // cannot send the same thing twice, and with a spinner, so a slow answer does
  // not look like a press that did nothing. A press while it is busy is
  // ignored. Answers what work answers.
  async busy( button , work ) {
    if ( !button ) return work();
    if ( button.getAttribute( "aria-busy" ) === "true" ) return undefined;
    const spinner = this.el( "span" , { class: "spinner-border spinner-border-sm me-2" , attrs: { "aria-hidden": "true" } } );
    button.disabled = true;
    button.setAttribute( "aria-busy" , "true" );
    button.prepend( spinner );
    try {
      return await work();
    } finally {
      spinner.remove();
      button.removeAttribute( "aria-busy" );
      button.disabled = false;
    }
  },

  // A form's submit handler, with its submit button busy until it finishes.
  // handler has to be async and to await what it sends, or the button comes
  // back early.
  onSubmit( form , handler ) {
    form.addEventListener( "submit" , ( event ) => {
      const button = event.submitter || form.querySelector( "[type=submit]" );
      if ( button && button.getAttribute( "aria-busy" ) === "true" ) {
        event.preventDefault();
        return;
      }
      this.busy( button , () => handler( event ) );
    } );
  },

  // Copy an element's text, with a selection fallback. The fallback is not
  // optional politeness: navigator.clipboard needs a secure context, so every
  // plain-http://localhost run lands there, and a copy button that silently
  // does nothing is worse than no button. Used for both login links and API
  // keys, which are exactly the strings nobody wants to retype by hand.
  async copy( source ) {
    const value = source ? source.textContent : "";
    try {
      await navigator.clipboard.writeText( value );
      this.toast( I18n.get( "ui.copied" ) , "ok" );
    } catch ( error ) {
      const range = document.createRange();
      range.selectNodeContents( source );
      const selection = window.getSelection();
      selection.removeAllRanges();
      selection.addRange( range );
      this.toast( I18n.get( "ui.copy_hint" ) , "ok" );
    }
  },

  // Saves text as a file, for what is better kept than copied.
  download( name , text ) {
    const url = URL.createObjectURL( new Blob( [ text ] , { type: "text/plain" } ) );
    const link = this.el( "a" , { attrs: { href: url , download: name } } );
    document.body.appendChild( link );
    link.click();
    link.remove();
    URL.revokeObjectURL( url );
  },

  // "3 hours ago", in the browser's own language. Past a month the date says
  // more than a count of days does.
  ago( date ) {
    const seconds = ( date.getTime() - Date.now() ) / 1000;
    const size = Math.abs( seconds );
    if ( size >= 30 * 86400 ) return date.toLocaleDateString();
    const format = new Intl.RelativeTimeFormat( undefined , { numeric: "auto" } );
    for ( const [ scale , unit ] of [ [ 86400 , "day" ] , [ 3600 , "hour" ] , [ 60 , "minute" ] ] ) {
      if ( size >= scale ) return format.format( Math.round( seconds / scale ) , unit );
    }
    return format.format( 0 , "second" );
  },

  // A time for a table cell: how long ago at a glance, the exact time on hover.
  // fallback is what a missing or unreadable time says instead ("Never").
  when( value , fallback ) {
    const parsed = new Date( value || "" );
    if ( isNaN( parsed.getTime() ) ) return this.el( "span" , { text: fallback || "" } );
    return this.el( "time" , {
      text: this.ago( parsed ),
      attrs: { datetime: parsed.toISOString() , title: parsed.toLocaleString() },
    } );
  },

  // Asks before something that cannot be taken back, in the app's own theme,
  // which window.confirm cannot be. Answers true or false. An empty question
  // answers true, so a language.yaml key set to "" stops the asking. Cancel
  // has the focus: Enter on a question nobody read should not delete anything.
  confirm( question ) {
    if ( !question ) return Promise.resolve( true );
    const element = this.get( "confirm-modal" );
    if ( element._open ) return Promise.resolve( false );
    element._open = true;
    const returnTo = document.activeElement;
    this.text( this.get( "confirm-text" ) , question );
    const modal = bootstrap.Modal.getOrCreateInstance( element );
    return new Promise( ( resolve ) => {
      let answer = false;
      const ok = this.get( "confirm-ok" );
      const onOk = () => {
        answer = true;
        modal.hide();
      };
      ok.addEventListener( "click" , onOk );
      element.addEventListener( "shown.bs.modal" , () => this.get( "confirm-cancel" ).focus() , { once: true } );
      element.addEventListener( "hidden.bs.modal" , () => {
        ok.removeEventListener( "click" , onOk );
        element._open = false;
        if ( returnTo && returnTo.focus ) returnTo.focus();
        resolve( answer );
      } , { once: true } );
      modal.show();
    } );
  },

  // Adds a show/hide button to a password field. A key set to "" leaves it out.
  revealable( input ) {
    const label = I18n.get( "ui.show_password" );
    if ( label === "" ) return;
    const icon = this.el( "i" , { class: "bi bi-eye" , attrs: { "aria-hidden": "true" } } );
    const set = ( shown ) => {
      input.type = shown ? "text" : "password";
      icon.className = "bi " + ( shown ? "bi-eye-slash" : "bi-eye" );
      button.setAttribute( "aria-pressed" , String( shown ) );
    };
    const button = this.el( "button" , {
      class: "btn btn-outline-secondary",
      attrs: { type: "button" , "aria-label": label , title: label , "aria-pressed": "false" },
      children: [ icon ],
      on: { click: () => set( input.type === "password" ) },
    } );
    const group = this.el( "div" , { class: "input-group" } );
    input.replaceWith( group );
    group.append( input , button );
    // Sent, it goes back to dots: whoever walks past later should not read it.
    if ( input.form ) input.form.addEventListener( "submit" , () => set( false ) );
  },

  // Briefly reveal a confirmation ("Saved.", "Copied.") without leaving it on
  // screen forever.
  flash( element , milliseconds ) {
    if ( !element ) return;
    element.hidden = false;
    window.clearTimeout( element._flashTimer );
    element._flashTimer = window.setTimeout( function () {
      element.hidden = true;
    } , milliseconds || 2000 );
  },

  // A short status line pinned to the bottom of the screen ("Saved.",
  // "Copied."), for confirmations that need no place of their own on the
  // page. kind is "ok" or "error". An empty message shows nothing, so a
  // language.yaml key set to "" switches that toast off.
  //
  // action is an optional { text , run }: a button on the toast ("Undo").
  // A toast with one stays up twice as long, since reaching for it takes
  // longer than reading it.
  //
  // The hiding is timed here rather than by Bootstrap, whose delay is fixed
  // when the instance is made; replacing the instance to change it breaks a
  // toast that is still fading out. The timer stops while the pointer or the
  // focus is on the toast, so it never disappears under someone aiming at it.
  toast( message , kind , action ) {
    const banner = this.get( "toast" );
    if ( !banner || !message ) return;
    banner.className = "toast border-0 " + ( kind === "error" ? "text-bg-danger" : "text-bg-success" );
    this.text( this.get( "toast-text" ) , message );
    const button = this.get( "toast-action" );
    Dom.show( button , Boolean( action ) );
    if ( action ) {
      this.text( button , action.text );
      // The button goes, the toast stays: what the action does next ("Undone:
      // ...", or an error) takes its place. Hiding it here would swallow that
      // next toast, which arrives while this one is still fading out.
      button.onclick = function () {
        Dom.show( button , false );
        action.run();
      };
    }
    const instance = bootstrap.Toast.getOrCreateInstance( banner , { autohide: false } );
    const schedule = function () {
      window.clearTimeout( banner._hideTimer );
      banner._hideTimer = window.setTimeout( function () { instance.hide(); } , banner._delay );
    };
    if ( !banner._wired ) {
      banner._wired = true;
      const hold = function () { window.clearTimeout( banner._hideTimer ); };
      const release = function () {
        if ( !banner.matches( ":hover" ) && !banner.contains( document.activeElement ) ) schedule();
      };
      banner.addEventListener( "mouseenter" , hold );
      banner.addEventListener( "focusin" , hold );
      banner.addEventListener( "mouseleave" , release );
      banner.addEventListener( "focusout" , () => window.setTimeout( release ) );
    }
    banner._delay = action ? 8000 : 4000;
    instance.show();
    schedule();
  },

  // A status pill for a table cell. tone is a Bootstrap colour name:
  // "success", "secondary", "warning", "danger".
  badge( text , tone ) {
    return this.el( "span" , { class: "badge text-bg-" + ( tone || "secondary" ) , text: text } );
  },
};
