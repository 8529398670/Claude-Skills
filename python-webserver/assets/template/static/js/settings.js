// whole-file feature:settings
// The Settings screen (owner only): config.yaml, one field per setting,
// grouped by section. app.js calls Settings.init( me ) for admins; only the
// owner gets past the first line, and the server answers anyone else with a
// 404 regardless.
//
// Everything on the screen comes from the server (GET /api/owner/settings):
// each key's type, value, default, help text and badge are worked out there
// from defaults.yaml's comments, so this file knows nothing about any one
// setting. A badge says when a change takes effect:
//
//   Live      at once
//   Restart   saved now, used when the server restarts
//   Scripts   read by docker/run.sh, not the server
//
// Saving sends only the fields that changed, with the version of the file
// they were loaded from. The server rewrites just those values in place --
// every comment in config.yaml stays -- and refuses with a 409 if the file
// changed since, so a hand edit made meanwhile is never overwritten. The
// secret key is not in config.yaml and never reaches this page; the screen
// only says which file holds it.
//
// The filter and the jump links only hide and scroll: a field the filter hides
// still counts as changed, and still saves. Use default fills the field in and
// marks it changed like typing would, so nothing is written until Save.
//
// A save is undoable (Ctrl+Z, or the toast's button): the undo puts the old
// values back through the same route, and is refused the same way if the
// file has moved on since.
const Settings = {
  refreshMs: 10000,
  timer: null,
  snapshot: null,
  // key -> { input , state , reset , row } for every field on screen.
  fields: {},
  // Each section's fieldset, its jump link, and its fields' rows.
  sections: [],

  async init( me ) {
    if ( !me.is_owner ) return;
    Dom.show( Dom.get( "settings-panel" ) , true );
    Dom.onSubmit( Dom.get( "settings-form" ) , async ( event ) => {
      event.preventDefault();
      await this.save();
    } );
    Dom.onPress( Dom.get( "settings-reload" ) , () => this.onReload() );
    Dom.get( "settings-filter" ).addEventListener( "input" , () => this.applyFilter() );
    window.addEventListener( "beforeunload" , ( event ) => {
      if ( this.changes().count > 0 ) event.preventDefault();
    } );
    await this.load();
    // Pick up hand edits while the screen is open, unless that would throw
    // away something typed here.
    this.timer = window.setInterval( () => {
      if ( Nav.current() === "settings" && !document.hidden && this.changes().count === 0 ) this.load( true );
    } , this.refreshMs );
  },

  // Reloading redraws the form, which throws away anything typed into it.
  async onReload() {
    if ( this.changes().count > 0 && !( await Dom.confirm( I18n.get( "settings.reload_confirm" ) ) ) ) return;
    await this.load();
  },

  // quiet: no error banner if it fails, and (unless force) no redraw when
  // nothing changed -- the background refresh.
  async load( quiet , force ) {
    let snapshot;
    try {
      snapshot = await Api.settings();
    } catch ( error ) {
      if ( !quiet ) App.showError( error.message );
      return;
    }
    const previous = this.snapshot;
    this.snapshot = snapshot;
    if ( quiet && !force && previous && previous.version === snapshot.version &&
         JSON.stringify( previous.error ) === JSON.stringify( snapshot.error ) &&
         previous.stale === snapshot.stale && previous.writable === snapshot.writable ) return;
    this.render();
  },

  editable() {
    return this.snapshot.writable && !this.snapshot.stale;
  },

  render() {
    const snapshot = this.snapshot;
    Dom.text( Dom.get( "settings-file" ) , I18n.get( "settings.file" ).replace( "{file}" , snapshot.file ) );
    Dom.text( Dom.get( "settings-secret" ) , I18n.get( "settings.secret_note" ).replace( "{path}" , snapshot.secret_file ) );

    const invalid = Dom.get( "settings-invalid" );
    Dom.text( invalid , snapshot.error ? I18n.get( "settings.invalid_file" ).replace( "{error}" , snapshot.error.message ) : "" );
    Dom.show( invalid , Boolean( snapshot.error ) );
    Dom.show( Dom.get( "settings-stale" ) , snapshot.stale );
    Dom.show( Dom.get( "settings-read-only" ) , !snapshot.writable && !snapshot.stale );
    Dom.show( Dom.get( "settings-changed" ) , false );
    Dom.show( Dom.get( "settings-problems" ) , false );

    const holder = Dom.get( "settings-sections" );
    const jump = Dom.get( "settings-jump" );
    Dom.clear( holder );
    Dom.clear( jump );
    this.fields = {};
    this.sections = [];
    snapshot.sections.forEach( ( section ) => {
      const block = Dom.el( "fieldset" , { class: "settings-section" , attrs: { tabindex: "-1" } } );
      block.appendChild( Dom.el( "legend" , { class: "h5" , text: section.name } ) );
      if ( section.help ) block.appendChild( Dom.el( "p" , { class: "form-text settings-help" , text: section.help } ) );
      const rows = section.keys.map( ( state ) => {
        const row = this.renderKey( state );
        block.appendChild( row );
        // What the filter looks through.
        return { row: row , text: [ section.name , state.key , state.name , state.help , this.format( state.value ) ].join( " " ).toLowerCase() };
      } );
      holder.appendChild( block );
      const link = Dom.el( "button" , {
        class: "btn btn-sm btn-outline-secondary",
        text: section.name,
        attrs: { type: "button" },
        on: { click: () => this.jumpTo( block ) },
      } );
      jump.appendChild( link );
      this.sections.push( { block: block , link: link , rows: rows } );
    } );
    this.applyFilter();
    this.updateSave();
  },

  // Every word has to match somewhere in the field or its section.
  applyFilter() {
    const words = Dom.get( "settings-filter" ).value.trim().toLowerCase().split( /\s+/ ).filter( Boolean );
    let shown = 0;
    let total = 0;
    this.sections.forEach( ( section ) => {
      const matches = section.rows.filter( ( item ) => {
        const match = words.every( ( word ) => item.text.includes( word ) );
        Dom.show( item.row , match );
        return match;
      } ).length;
      Dom.show( section.block , matches > 0 );
      Dom.show( section.link , matches > 0 );
      shown += matches;
      total += section.rows.length;
    } );
    let status = "";
    if ( words.length > 0 ) {
      status = shown === 0 ? I18n.get( "ui.no_matches" )
        : I18n.get( "settings.filter_count" ).replace( "{count}" , shown ).replace( "{total}" , total );
    }
    Dom.text( Dom.get( "settings-filter-status" ) , status );
  },

  clearFilter() {
    Dom.get( "settings-filter" ).value = "";
    this.applyFilter();
  },

  // By script rather than a #link, because the hash says which screen this is.
  // Measured, because the tools above stay on screen and can wrap.
  jumpTo( block ) {
    const tools = document.querySelector( ".settings-tools" );
    const sticky = getComputedStyle( tools ).position === "sticky";
    const offset = document.querySelector( ".app-navbar" ).getBoundingClientRect().bottom + ( sticky ? tools.offsetHeight : 0 ) + 8;
    // Instant: the page scrolls smoothly by default, too slowly over this length.
    Dom.movedOnPurpose();
    window.scrollTo( { top: window.scrollY + block.getBoundingClientRect().top - offset , behavior: "instant" } );
    block.focus( { preventScroll: true } );
  },

  renderKey( state ) {
    const id = "setting-" + state.key.replace( /[^a-z0-9]+/gi , "-" );
    const disabled = state.locked || !this.editable();
    let input;
    if ( state.type === "bool" ) {
      input = Dom.el( "input" , { class: "form-check-input" , attrs: { type: "checkbox" , role: "switch" , id: id } } );
      input.checked = Boolean( state.value );
    } else if ( state.type === "list" ) {
      input = Dom.el( "textarea" , { class: "form-control font-monospace" , attrs: { id: id , rows: String( Math.max( 2 , state.value.length + 1 ) ) } } );
      input.value = state.value.join( "\n" );
    } else {
      input = Dom.el( "input" , { class: "form-control" , attrs: {
        id: id,
        type: state.type === "int" ? "number" : "text",
        step: "1",
        autocomplete: "off",
        spellcheck: "false",
      } } );
      input.value = String( state.value );
    }
    input.disabled = disabled;
    input.addEventListener( "input" , () => this.updateSave() );
    input.addEventListener( "change" , () => this.updateSave() );
    const field = this.fields[ state.key ] = { input: input , state: state , reset: null };

    const tone = { live: "success" , restart: "warning" , scripts: "secondary" }[ state.apply ] || "secondary";
    const label = Dom.el( "label" , { class: "form-label settings-label d-flex flex-wrap align-items-center gap-2" , attrs: { for: id } , children: [
      Dom.el( "code" , { text: state.name } ),
      Dom.badge( I18n.get( "settings.apply_" + state.apply ) || state.apply , tone ),
    ] } );
    label.lastChild.setAttribute( "title" , I18n.get( "settings.apply_" + state.apply + "_help" ) );

    const notes = Dom.el( "div" , { class: "form-text" } );
    if ( state.help ) notes.appendChild( Dom.el( "div" , { class: "settings-help" , text: state.help } ) );
    if ( state.type === "list" ) notes.appendChild( Dom.el( "div" , { text: I18n.get( "settings.list_help" ) } ) );
    const fallback = Dom.el( "div" , { text: I18n.get( "settings.default" ).replace( "{value}" , this.format( state.default ) ) } );
    if ( !disabled && I18n.get( "settings.reset_button" ) !== "" ) {
      const reset = I18n.get( "settings.reset_label" ).replace( "{name}" , () => state.name );
      field.reset = Dom.el( "button" , {
        class: "btn btn-link btn-sm settings-reset",
        text: I18n.get( "settings.reset_button" ),
        attrs: { type: "button" , "aria-label": reset , title: reset },
        on: { click: () => this.useDefault( field ) },
      } );
      fallback.append( " " , field.reset );
    }
    notes.appendChild( fallback );
    if ( state.pending_restart ) {
      notes.appendChild( Dom.el( "div" , { class: "text-warning-emphasis" , text: I18n.get( "settings.pending_restart" ).replace( "{running}" , this.format( state.running ) ) } ) );
    }
    if ( state.file_only ) notes.appendChild( Dom.el( "div" , { text: I18n.get( "settings.file_only" ) } ) );
    else if ( state.locked ) notes.appendChild( Dom.el( "div" , { text: I18n.get( "settings.locked" ) } ) );

    const row = Dom.el( "div" , { class: "settings-key" , attrs: { "data-key": state.key } } );
    if ( state.type === "bool" ) {
      row.appendChild( label );
      row.appendChild( Dom.el( "div" , { class: "form-check form-switch" , children: [ input ] } ) );
    } else {
      row.appendChild( label );
      row.appendChild( input );
    }
    row.appendChild( notes );
    return row;
  },

  // Fills the field in with its default. The button goes as the value matches,
  // so the focus moves to the field.
  useDefault( field ) {
    const value = field.state.default;
    if ( field.state.type === "bool" ) field.input.checked = Boolean( value );
    else if ( field.state.type === "list" ) field.input.value = ( value || [] ).join( "\n" );
    else field.input.value = String( value );
    Dom.clearFieldError( field.input );
    this.updateSave();
    field.input.focus();
  },

  format( value ) {
    if ( Array.isArray( value ) ) return value.length ? value.join( ", " ) : I18n.get( "settings.empty" );
    if ( value === "" ) return I18n.get( "settings.empty" );
    return String( value );
  },

  read( field ) {
    const input = field.input;
    switch ( field.state.type ) {
      case "bool": return input.checked;
      case "int": return input.value.trim() === "" ? NaN : Number( input.value );
      case "list": return input.value.split( "\n" ).map( ( line ) => line.trim() ).filter( Boolean );
      default: return input.value;
    }
  },

  // { values: { key: new } , previous: { key: old } , count }
  changes() {
    const result = { values: {} , previous: {} , count: 0 };
    Object.entries( this.fields ).forEach( ( entry ) => {
      const field = entry[ 1 ];
      const value = this.read( field );
      const changed = JSON.stringify( value ) !== JSON.stringify( field.state.value );
      field.input.classList.toggle( "settings-changed" , changed );
      if ( field.reset ) Dom.show( field.reset , JSON.stringify( value ) !== JSON.stringify( field.state.default ) );
      if ( !changed ) return;
      result.values[ entry[ 0 ] ] = value;
      result.previous[ entry[ 0 ] ] = field.state.value;
      result.count += 1;
    } );
    return result;
  },

  updateSave() {
    const count = this.changes().count;
    Dom.get( "settings-save" ).disabled = count === 0 || !this.editable();
    Dom.text( Dom.get( "settings-unsaved" ) , count ? I18n.get( "settings.unsaved" ).replace( "{count}" , count ) : "" );
  },

  showProblems( problems ) {
    const general = [];
    ( problems || [] ).forEach( ( problem ) => {
      // Each problem starts with the key it is about.
      const key = Object.keys( this.fields ).filter( ( name ) => problem.startsWith( name ) ).sort( ( a , b ) => b.length - a.length )[ 0 ];
      if ( key ) {
        Dom.fieldError( this.fields[ key ].input , problem );
      } else {
        general.push( problem );
      }
    } );
    // An error the filter is hiding would look like a save that did nothing.
    if ( Object.values( this.fields ).some( ( field ) => field.input.classList.contains( "is-invalid" ) && field.input.offsetParent === null ) ) this.clearFilter();
    const box = Dom.get( "settings-problems" );
    Dom.text( box , general.join( "\n" ) );
    Dom.show( box , general.length > 0 );
    const first = document.querySelector( "#settings-sections .is-invalid" );
    if ( first ) first.focus();
  },

  async save() {
    const pending = this.changes();
    if ( pending.count === 0 ) return;
    Object.values( this.fields ).forEach( ( field ) => Dom.clearFieldError( field.input ) );
    let result;
    try {
      result = await this.apply( this.snapshot.version , pending.values );
    } catch ( error ) {
      this.showSaveError( error );
      return;
    }
    Undo.saved( Dom.get( "settings-form" ) );
    // Each direction expects the file the other one left behind, so an undo
    // after a hand edit is refused (409) rather than overwriting it.
    const versions = { after: result.version , before: null };
    const restart = ( result.restart || [] ).length > 0;
    Dom.toast( I18n.get( restart ? "settings.saved_restart" : "settings.saved" ) , "ok" );
    Undo.record( {
      label: I18n.get( "undo.settings" ),
      toast: false,
      undo: async () => { versions.before = ( await this.apply( versions.after , pending.previous ) ).version; },
      redo: async () => { versions.after = ( await this.apply( versions.before , pending.values ) ).version; },
    } );
  },

  async apply( version , values ) {
    const result = await Api.saveSettings( version , values );
    // The file holds these values now. Show that straight away, so a refresh
    // that fails (a rate limit just lowered, say) cannot leave them looking
    // unsaved; the refresh then brings in what else the server worked out.
    this.snapshot.version = result.version;
    this.snapshot.sections.forEach( ( section ) => section.keys.forEach( ( state ) => {
      if ( Object.prototype.hasOwnProperty.call( values , state.key ) ) state.value = values[ state.key ];
    } ) );
    this.render();
    await this.load( true , true );
    return result;
  },

  showSaveError( error ) {
    const payload = error.payload || {};
    if ( error.status === 409 && payload.error === "settings_changed" ) {
      Dom.show( Dom.get( "settings-changed" ) , true );
      return;
    }
    if ( error.status === 400 && payload.problems ) {
      this.showProblems( payload.problems );
      return;
    }
    App.showError( error.message );
  },
};
