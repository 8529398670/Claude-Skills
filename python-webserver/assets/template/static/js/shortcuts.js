// whole-file feature:shortcuts
// Every keyboard shortcut in the app, and the Shortcuts screen where each
// person changes theirs.
//
// One list of actions, each with its default keys. Someone's own choices come
// from /api/me ("shortcuts") and are saved to their account, so they follow
// them to every browser. Only what they changed is stored; anything else uses
// its default.
//
// A key is written the same way on every machine: "Mod+Shift+Z". Mod is Cmd
// on a Mac and Ctrl on Windows and Linux, so one saved choice works on all
// three. Shortcuts.os() works out which one this browser runs on, and the keys
// are shown the way that machine labels them (⌘⇧Z on a Mac, Ctrl+Shift+Z
// elsewhere). Modifiers are always in the order Mod, Ctrl, Alt, Shift; Ctrl on
// its own means the Mac's Control key, and is the same as Mod elsewhere.
//
// An app adds its own with Shortcuts.register, before App.init runs:
//
//   Shortcuts.register( { id: "new_card" , label: "shortcuts.new_card" ,
//                         keys: [ "N" ] , run: () => Board.newCard() } );
//
// It is listed on the Shortcuts screen and in the ? list, and can be changed
// like the rest. keys can differ by OS: keys: { mac: [ ... ] , other: [ ... ] }.
//
// Where an action applies (context):
//   page    -- anywhere, except while someone is typing in a field or a
//              dialog is open. Most shortcuts.
//   editing -- while a name is being edited in place (Dom.editable asks
//              Shortcuts.matches; nothing here runs it).
//   dialog  -- while a dialog is open. Esc there is also Bootstrap's own, so it
//              keeps closing dialogs whatever close_dialog is set to.
// Two actions may share keys only in different contexts.
//
// Setting shortcuts.heading to "" in language.yaml leaves the screen, the
// list and its button out. The keys still work, with their defaults.
const Shortcuts = {
  actions: [],
  // What this person changed: action id -> key, "" for switched off.
  mine: {},
  platform: null,
  // While the Shortcuts screen waits for new keys: { id , done }.
  capturing: null,
  // The row whose button gets the focus after the next redraw.
  refocus: null,
  // Redraws when the keys change: the screen and the ? list.
  onChange: [],

  register( action ) {
    const existing = this.actions.findIndex( ( other ) => other.id === action.id );
    const entry = Object.assign( { context: "page" } , action );
    if ( existing >= 0 ) this.actions[ existing ] = entry;
    else this.actions.push( entry );
  },

  // "mac", "windows" or "linux". userAgentData where the browser has it,
  // navigator.platform where it does not; an Android phone or a Chromebook is
  // "linux", which labels its keys the same way.
  os() {
    if ( this.platform ) return this.platform;
    const data = navigator.userAgentData;
    const name = ( ( data && data.platform ) || navigator.platform || navigator.userAgent || "" ).toLowerCase();
    this.platform = /mac|iphone|ipad|ipod/.test( name ) ? "mac" : /win/.test( name ) ? "windows" : "linux";
    return this.platform;
  },

  mac() {
    return this.os() === "mac";
  },

  find( id ) {
    return this.actions.find( ( action ) => action.id === id ) || null;
  },

  defaults( action ) {
    const keys = action.keys || [];
    if ( Array.isArray( keys ) ) return keys;
    return ( this.mac() ? keys.mac : keys.other ) || [];
  },

  // The keys an action answers to right now: its own, or its defaults.
  keys( id ) {
    const action = this.find( id );
    if ( !action ) return [];
    if ( Object.prototype.hasOwnProperty.call( this.mine , id ) ) {
      return this.mine[ id ] === "" ? [] : [ this.mine[ id ] ];
    }
    return this.defaults( action );
  },

  changed( id ) {
    return Object.prototype.hasOwnProperty.call( this.mine , id );
  },

  // Off a Mac there is no separate Control key: Ctrl is Mod.
  normal( binding ) {
    if ( this.mac() || !binding ) return binding;
    const parts = binding.split( /\+(?=.)/ );
    const key = parts.pop();
    const mods = [ "Mod" , "Alt" , "Shift" ].filter( ( mod ) =>
      parts.includes( mod ) || ( mod === "Mod" && parts.includes( "Ctrl" ) ) );
    return mods.concat( [ key ] ).join( "+" );
  },

  // What was pressed, written the way bindings are, or null for a modifier
  // on its own (or anything else no shortcut can be).
  fromEvent( event ) {
    let key = event.key;
    if ( !key || [ "Shift" , "Control" , "Alt" , "Meta" , "OS" , "CapsLock" , "Dead" , "Unidentified" , "Process" , "Compose" , "AltGraph" ].includes( key ) ) {
      return null;
    }
    // Option+K on a Mac types ˚: the key on the keyboard is what was meant.
    if ( event.altKey && event.code ) {
      const letter = /^Key([A-Z])$/.exec( event.code ) || /^Digit([0-9])$/.exec( event.code );
      if ( letter ) key = letter[ 1 ];
    }
    if ( key === " " ) key = "Space";
    if ( key.length === 1 ) key = key.toUpperCase();
    const mac = this.mac();
    const parts = [];
    if ( mac ? event.metaKey : event.ctrlKey ) parts.push( "Mod" );
    if ( mac && event.ctrlKey ) parts.push( "Ctrl" );
    if ( event.altKey ) parts.push( "Alt" );
    // ? is already Shift and /, so Shift is only written for letters and
    // named keys, where it is the only sign of it.
    const character = key.length === 1;
    const letter = character && key.toLowerCase() !== key.toUpperCase();
    if ( event.shiftKey && ( !character || letter ) ) parts.push( "Shift" );
    // The Windows key is the system's, not the page's.
    if ( !mac && event.metaKey ) return null;
    parts.push( key );
    return parts.join( "+" );
  },

  matches( id , event ) {
    const pressed = this.fromEvent( event );
    if ( !pressed ) return false;
    return this.keys( id ).some( ( binding ) => this.normal( binding ) === pressed );
  },

  // A plain character is typing, wherever the focus is.
  isTyping( binding ) {
    const parts = binding.split( /\+(?=.)/ );
    const key = parts.pop();
    return key.length === 1 && !parts.some( ( mod ) => mod === "Mod" || mod === "Ctrl" || mod === "Alt" );
  },

  // Key caps for a binding, the way this machine labels them.
  caps( binding ) {
    const mac = this.mac();
    const parts = this.normal( binding ).split( /\+(?=.)/ );
    const names = {
      Escape: "Esc", Enter: mac ? "Return" : "Enter", Space: "Space", Backspace: mac ? "Delete" : "Backspace",
      Delete: mac ? "⌦" : "Delete", ArrowUp: "↑", ArrowDown: "↓", ArrowLeft: "←", ArrowRight: "→", Tab: "Tab",
    };
    const mods = mac ? { Mod: "⌘", Ctrl: "⌃", Alt: "⌥", Shift: "⇧" } : { Mod: "Ctrl", Ctrl: "Ctrl", Alt: "Alt", Shift: "Shift" };
    return parts.map( ( part , index ) => index < parts.length - 1 ? mods[ part ] || part : names[ part ] || part );
  },

  // <kbd> per key; a group per binding, "or" between them.
  keysElement( bindings ) {
    const span = Dom.el( "span" , { class: "shortcut-keys" } );
    if ( bindings.length === 0 ) {
      span.appendChild( Dom.el( "span" , { class: "text-body-secondary" , text: I18n.get( "shortcuts.off" ) } ) );
      return span;
    }
    bindings.forEach( ( binding , index ) => {
      if ( index > 0 ) span.appendChild( Dom.el( "span" , { class: "shortcut-or" , text: I18n.get( "shortcuts.or" ) } ) );
      const group = Dom.el( "span" , { class: "shortcut-combo" , attrs: { "aria-label": this.caps( binding ).join( " " ) } } );
      this.caps( binding ).forEach( ( cap ) => group.appendChild( Dom.el( "kbd" , { text: cap } ) ) );
      span.appendChild( group );
    } );
    return span;
  },

  label( action ) {
    return I18n.get( action.label ) || action.id;
  },

  listed() {
    return I18n.get( "shortcuts.heading" ) !== "";
  },

  // ---- Running them ----

  init() {
    if ( !this.listed() ) {
      Dom.show( Dom.get( "shortcuts" ) , false );
      return;
    }
    Dom.get( "shortcuts-list-change" ).addEventListener( "click" , () => {
      bootstrap.Modal.getOrCreateInstance( Dom.get( "shortcuts-modal" ) ).hide();
    } );
    Dom.get( "shortcuts-reset-all" ).addEventListener( "click" , () => this.resetAll() );
    this.onChange.push( () => this.renderList() );
    this.renderList();
  },

  // From App.renderSignedIn: this person's own keys, and their screen.
  signedIn( me ) {
    this.mine = Object.assign( {} , me.shortcuts || {} );
    if ( !this.listed() ) return;
    const os = I18n.get( "shortcuts.os_" + this.os() );
    Dom.text( Dom.get( "shortcuts-detected" ) , I18n.get( "shortcuts.detected" ).replace( "{os}" , os ) );
    this.onChange.push( () => this.renderScreen() );
    this.changedKeys();
  },

  changedKeys() {
    this.onChange.forEach( ( redraw ) => redraw() );
  },

  onKey( event ) {
    if ( event.isComposing || this.capturing || event.defaultPrevented ) return;
    const pressed = this.fromEvent( event );
    if ( !pressed ) return;
    const typing = Undo.typing( event.target );
    const dialog = document.querySelector( ".modal.show" );
    const context = dialog ? "dialog" : "page";
    // While typing only a dialog's keys count, and only ones that do not type.
    if ( typing && context === "page" ) return;
    for ( const action of this.actions ) {
      if ( action.context !== context || !action.run ) continue;
      const hit = this.keys( action.id ).find( ( binding ) => this.normal( binding ) === pressed );
      if ( !hit ) continue;
      if ( typing && this.isTyping( hit ) ) return;
      if ( action.run( event , dialog ) === false ) continue;
      event.preventDefault();
      return;
    }
  },

  showList() {
    if ( !this.listed() ) return;
    const element = Dom.get( "shortcuts-modal" );
    const returnTo = document.activeElement;
    element.addEventListener( "hidden.bs.modal" , () => {
      if ( returnTo && returnTo.focus && returnTo.isConnected ) returnTo.focus();
    } , { once: true } );
    bootstrap.Modal.getOrCreateInstance( element ).show();
  },

  focusSearch() {
    const box = Dom.all( "input[type=search]" ).find( ( input ) => input.offsetParent !== null );
    if ( !box ) return false;
    box.focus();
    box.select();
    return true;
  },

  // ---- The ? list ----

  renderList() {
    const list = Dom.get( "shortcut-list" );
    Dom.clear( list );
    this.actions.forEach( ( action ) => {
      const label = this.label( action );
      if ( label === "" ) return;
      list.appendChild( Dom.el( "dt" , { children: [ this.keysElement( this.keys( action.id ) ) ] } ) );
      list.appendChild( Dom.el( "dd" , { text: label } ) );
    } );
    // Someone signed out has no screen to change them on.
    Dom.show( Dom.get( "shortcuts-list-change" ) , Boolean( App.currentUser ) );
  },

  // ---- The Shortcuts screen ----

  renderScreen() {
    const body = Dom.get( "shortcuts-rows" );
    Dom.clear( body );
    this.actions.forEach( ( action ) => {
      const label = this.label( action );
      if ( label === "" ) return;
      const keysCell = Dom.el( "td" , { children: [ this.keysElement( this.keys( action.id ) ) ] } );
      const buttons = [
        Dom.el( "button" , {
          class: "btn btn-sm btn-outline-primary",
          text: I18n.get( "shortcuts.change" ),
          attrs: { type: "button" , "aria-label": I18n.get( "shortcuts.change_named" ).replace( "{name}" , label ) },
          on: { click: () => this.capture( action ) },
        } ),
      ];
      if ( this.keys( action.id ).length > 0 ) {
        buttons.push( Dom.el( "button" , {
          class: "btn btn-sm btn-outline-secondary",
          text: I18n.get( "shortcuts.turn_off" ),
          attrs: { type: "button" , "aria-label": I18n.get( "shortcuts.turn_off_named" ).replace( "{name}" , label ) },
          on: { click: () => this.set( action.id , "" ) },
        } ) );
      }
      if ( this.changed( action.id ) ) {
        buttons.push( Dom.el( "button" , {
          class: "btn btn-sm btn-link",
          text: I18n.get( "shortcuts.reset" ),
          attrs: { type: "button" , "aria-label": I18n.get( "shortcuts.reset_named" ).replace( "{name}" , label ) },
          on: { click: () => this.set( action.id , null ) },
        } ) );
      }
      body.appendChild( Dom.el( "tr" , {
        attrs: { "data-shortcut": action.id },
        children: [
          Dom.el( "td" , { text: label } ),
          keysCell,
          Dom.el( "td" , { class: "text-end" , children: [ Dom.el( "div" , { class: "row-actions shortcut-actions" , children: buttons } ) ] } ),
        ],
      } ) );
    } );
    Pager.label( "shortcuts-table" );
    Dom.show( Dom.get( "shortcuts-reset-all" ) , Object.keys( this.mine ).length > 0 );
    // A redraw takes the pressed button away: the focus goes back to its row.
    if ( this.refocus ) {
      const button = document.querySelector( '[data-shortcut="' + this.refocus + '"] button' );
      this.refocus = null;
      if ( button ) button.focus();
    }
  },

  // Waits for the next keys pressed, then sets them. Esc on its own gives up,
  // and so does a click anywhere; Reset is the way back to a default of Esc.
  capture( action ) {
    if ( this.capturing ) this.capturing.done( false );
    this.renderScreen();
    const cell = document.querySelector( '[data-shortcut="' + action.id + '"] td:nth-child(2)' );
    if ( !cell ) return;
    Dom.clear( cell );
    cell.appendChild( Dom.el( "span" , { class: "shortcut-capturing" , attrs: { role: "status" } , text: I18n.get( "shortcuts.press_keys" ) } ) );
    let finished = false;
    const listener = ( event ) => {
      if ( event.isComposing ) return;
      const pressed = this.fromEvent( event );
      event.preventDefault();
      event.stopPropagation();
      if ( !pressed ) return;
      done();
      if ( pressed !== "Escape" ) this.choose( action , pressed );
    };
    // After the click has done its own thing, which may be another Change.
    const cancel = () => window.setTimeout( () => done() , 0 );
    const done = ( redraw = true ) => {
      if ( finished ) return;
      finished = true;
      document.removeEventListener( "keydown" , listener , true );
      document.removeEventListener( "click" , cancel , true );
      if ( this.capturing && this.capturing.done === done ) this.capturing = null;
      if ( !redraw ) return;
      this.refocus = action.id;
      this.renderScreen();
    };
    this.capturing = { id: action.id , done: done };
    document.addEventListener( "keydown" , listener , true );
    document.addEventListener( "click" , cancel , true );
  },

  // Keys already used by something else in the same place: offer to swap.
  async choose( action , binding ) {
    if ( this.keys( action.id ).some( ( own ) => this.normal( own ) === binding ) && this.keys( action.id ).length === 1 ) return;
    const clash = this.actions.find( ( other ) => other.id !== action.id && other.context === action.context &&
      this.keys( other.id ).some( ( theirs ) => this.normal( theirs ) === binding ) );
    if ( !clash ) return this.set( action.id , binding );
    const question = I18n.get( "shortcuts.conflict" )
      .replaceAll( "{keys}" , this.caps( binding ).join( this.mac() ? "" : "+" ) )
      .replaceAll( "{other}" , this.label( clash ) )
      .replaceAll( "{name}" , this.label( action ) );
    if ( !( await Dom.confirm( question ) ) ) return;
    this.refocus = action.id;
    // The other one takes over this one's keys, or none.
    const given = this.keys( action.id ).find( ( own ) => this.normal( own ) !== binding ) || "";
    const next = this.with( this.with( this.mine , action.id , binding ) , clash.id , given );
    return this.save( next );
  },

  // A copy of the changed keys with one action's set: null puts it back to its
  // default, and so does choosing exactly its default.
  with( mine , id , binding ) {
    const next = Object.assign( {} , mine );
    const action = this.find( id );
    const defaults = action ? this.defaults( action ) : [];
    if ( binding === null || ( defaults.length === 1 && binding !== "" && this.normal( defaults[ 0 ] ) === this.normal( binding ) ) ) {
      delete next[ id ];
    } else {
      next[ id ] = binding;
    }
    return next;
  },

  set( id , binding ) {
    this.refocus = id;
    return this.save( this.with( this.mine , id , binding ) );
  },

  resetAll() {
    return this.save( {} );
  },

  save( next ) {
    const previous = Object.assign( {} , this.mine );
    return Undo.run( {
      label: I18n.get( "undo.shortcuts" ),
      do: () => this.store( next ),
      undo: () => this.store( previous ),
    } );
  },

  async store( next ) {
    const reply = await Api.setShortcuts( next );
    this.mine = Object.assign( {} , reply.shortcuts || {} );
    this.changedKeys();
  },
};

// The ones the template ships with. Labels are language.yaml keys; one set to
// "" is left off the lists but keeps working.
Shortcuts.register( { id: "search" , label: "shortcuts.search" , keys: [ "/" ] ,
  run: () => ( Shortcuts.focusSearch() ? undefined : false ) } );
Shortcuts.register( { id: "help" , label: "shortcuts.help" , keys: [ "?" ] ,
  run: () => ( Shortcuts.listed() ? Shortcuts.showList() : false ) } );
Shortcuts.register( { id: "undo" , label: "shortcuts.undo" , keys: [ "Mod+Z" ] , run: () => { Undo.undo(); } } );
Shortcuts.register( { id: "redo" , label: "shortcuts.redo" ,
  keys: { mac: [ "Mod+Shift+Z" ] , other: [ "Mod+Y" , "Mod+Shift+Z" ] } , run: () => { Undo.redo(); } } );
Shortcuts.register( { id: "edit_save" , label: "shortcuts.edit_save" , keys: [ "Enter" ] , context: "editing" } );
Shortcuts.register( { id: "edit_cancel" , label: "shortcuts.edit_cancel" , keys: [ "Escape" ] , context: "editing" } );
// Bootstrap closes a dialog on Esc itself; this is for any other keys chosen.
Shortcuts.register( { id: "close_dialog" , label: "shortcuts.close_dialog" , keys: [ "Escape" ] , context: "dialog" ,
  run: ( event , dialog ) => {
    if ( Shortcuts.fromEvent( event ) === "Escape" ) return false;
    bootstrap.Modal.getOrCreateInstance( dialog ).hide();
  } } );

document.addEventListener( "keydown" , ( event ) => Shortcuts.onKey( event ) );
