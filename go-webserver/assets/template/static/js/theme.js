// How the page looks: light, dark, or one of a few ready-made themes.
//
// Light is the default and stays it -- never the OS setting. Picking another
// theme is remembered in this browser (localStorage) only; nothing goes to
// the server. Move it onto the account if a theme should follow a person to
// every device.
//
// Dark is Bootstrap's own dark mode (<html data-bs-theme="dark">). Light is
// the attribute being absent, and Bootstrap's colour-mode script, which would
// follow prefers-color-scheme, is deliberately not included.
//
// This file is loaded in <head>, before the stylesheet's first paint, so a
// saved dark theme never flashes light on the way in.
// >>> feature:appearance
//
// Brand colours live in ThemePalettes, named, whether a preset uses them or not,
// so the rest are there for whoever needs them next.
const ThemePalettes = {
  // Wright State University. Primary: green and gold. The rest are its accents.
  wsu: {
    green: "#046a38",
    gold: "#cba052",
    dark_green: "#00482b",
    deep_gold: "#ae8643",
    yellow: "#f8e08e",
    leaf: "#719949",
    beige: "#dcd59a",
    black: "#101820",
    faded_gold: "#e7e3c6",
    off_white: "#f2f1f0",
  },
};

// <<< feature:appearance
const Theme = {
  storageKey: "theme",
  // What redraws when the theme changes: the Appearance picker. // feature:appearance
  // What redraws when the theme changes: the navbar button. // feature:!appearance
  onChange: [],

  // Each preset is a mode and two colours, plus its own page and card colours if
  // the mode's will not do. accent is the main colour: what you can press. second
  // says where you are: the active nav item, headings, progress, info. The rest
  // is worked out in colours() below. Names come from language.yaml
  // (theme.preset_<key>).
  presets: [
    { key: "light" , mode: "light" , accent: "#1f5fd6" , second: "#0b7a75" },
    { key: "dark" , mode: "dark" , accent: "#6ea0ff" , second: "#5fd3c4" },
    // >>> feature:appearance
    { key: "ocean" , mode: "light" , accent: "#0b7285" , second: "#bd5418" , background: "#eef6f8" , surface: "#ffffff" },
    { key: "forest" , mode: "light" , accent: "#28843b" , second: "#8a6a12" , background: "#f1f6ef" , surface: "#ffffff" },
    { key: "sepia" , mode: "light" , accent: "#9c5a1c" , second: "#2f6f6a" , background: "#f6f0e4" , surface: "#fbf7ef" },
    { key: "grape" , mode: "light" , accent: "#7048c8" , second: "#c0267e" , background: "#f5f2fc" , surface: "#ffffff" },
    { key: "pink" , mode: "light" , accent: "#c2255c" , second: "#6b3fc0" , background: "#fdf0f5" , surface: "#ffffff" },
    // Wright State University: Green on Off White, and Gold on Black with Dark Green panels.
    { key: "wsu" , mode: "light" , accent: ThemePalettes.wsu.green , second: ThemePalettes.wsu.gold , background: ThemePalettes.wsu.off_white , surface: "#ffffff" },
    { key: "midnight" , mode: "dark" , accent: "#8c7cff" , second: "#ffb86b" , background: "#0e1020" , surface: "#171a30" },
    { key: "slate" , mode: "dark" , accent: "#4fb3a9" , second: "#e6b450" , background: "#15191e" , surface: "#1d232a" },
    { key: "dark_pink" , mode: "dark" , accent: "#f06595" , second: "#ffa8d0" , background: "#1f0d16" , surface: "#2a1220" },
    { key: "wsu_dark" , mode: "dark" , accent: ThemePalettes.wsu.gold , second: ThemePalettes.wsu.yellow , background: ThemePalettes.wsu.black , surface: ThemePalettes.wsu.dark_green },
    // <<< feature:appearance
  ],

  // What a preset leaves out.
  modes: {
    light: { background: "#f7f8fa" , surface: "#ffffff" , text: "#212529" },
    dark: { background: "#101215" , surface: "#191c21" , text: "#dee2e6" },
  },

  // How much of the second colour goes into each layer, in percent, so a nested
  // card stands off its card and the card off the page.
  tints: { page: 3.5 , surface: 1.75 , nested: 8.75 , chrome: 10 , info: 14 , infoBorder: 45 },

  // Field of colours() -> custom property in style.css, which hands them on to
  // Bootstrap.
  vars: {
    accent: "--app-accent",
    accentFg: "--app-accent-fg",
    second: "--app-second",
    secondFg: "--app-second-fg",
    secondText: "--app-second-text",
    link: "--app-link",
    navLink: "--app-nav-link",
    background: "--app-bg",
    surface: "--app-surface",
    nested: "--app-nested",
    chrome: "--app-chrome",
    info: "--app-info",
    infoBorder: "--app-info-border",
    border: "--app-border",
  },

  current: "light",
  // WCAG AA for text. scripts/check_themes.js holds every preset to it.
  minContrast: 4.5,

  find( key ) {
    return this.presets.find( function ( preset ) { return preset.key === key; } ) || this.presets[ 0 ];
  },

  restore() {
    let saved = "";
    try {
      saved = window.localStorage.getItem( this.storageKey ) || "";
    } catch ( error ) {
      // Storage blocked (private window, previews): stay light.
    }
    this.current = this.find( saved ).key;
    this.apply( this.find( this.current ) );
  },

  set( key ) {
    this.current = this.find( key ).key;
    try {
      if ( this.current === "light" ) window.localStorage.removeItem( this.storageKey );
      else window.localStorage.setItem( this.storageKey , this.current );
    } catch ( error ) {
      // Not remembered for next time, but still applied now.
    }
    this.apply( this.find( this.current ) );
  },

  apply( preset ) {
    const root = document.documentElement;
    if ( preset.mode === "dark" ) root.setAttribute( "data-bs-theme" , "dark" );
    else root.removeAttribute( "data-bs-theme" );
    const colours = this.colours( preset );
    Object.keys( this.vars ).forEach( ( field ) => root.style.setProperty( this.vars[ field ] , colours[ field ] ) );
  },

  // Every colour the page needs, from a preset's few. Fills keep the colour as
  // chosen. Anything read as text is pushed until it is readable, on the nested
  // card, the most tinted place it can land: WSU's gold is gold on a progress bar
  // and darker in a heading.
  colours( preset ) {
    const mode = this.modes[ preset.mode ];
    const background = preset.background || mode.background;
    const surface = preset.surface || mode.surface;
    const second = preset.second;
    const card = this.mix( second , surface , this.tints.surface );
    const nested = this.mix( second , surface , this.tints.nested );
    const chrome = this.mix( second , surface , this.tints.chrome );
    return {
      text: mode.text,
      accent: preset.accent,
      accentFg: this.readableOn( preset.accent ),
      second: second,
      secondFg: this.readableOn( second ),
      secondText: this.asText( second , nested ),
      link: this.asText( preset.accent , nested ),
      navLink: this.asText( preset.accent , chrome ),
      background: this.mix( second , background , this.tints.page ),
      surface: card,
      nested: nested,
      chrome: chrome,
      info: this.mix( second , card , this.tints.info ),
      infoBorder: this.mix( second , card , this.tints.infoBorder ),
      // From the card, not the page: most borders sit on a card, and a card can be
      // a long way from its page (WSU dark's green on black).
      border: this.mix( mode.text , card , 20 ),
    };
  },

  // Picking is undoable, and quick picks merge into one step.
  pick( key ) {
    const previous = this.current;
    if ( previous === key ) return;
    const choose = ( chosen ) => {
      this.set( chosen );
      this.onChange.forEach( ( listener ) => listener() );
    };
    Undo.run( {
      label: I18n.get( "undo.theme" ),
      merge: "theme",
      screen: null,
      do: () => choose( key ),
      undo: () => choose( previous ),
    } );
  },

  // >>> feature:!appearance
  // The button in the navbar, for an app without the Appearance screen: light
  // or dark from anywhere, signed in or not. Dark from any light theme, light
  // from any dark one.
  watchToggle( button ) {
    const icon = button.querySelector( "i" );
    const update = () => {
      const dark = this.find( this.current ).mode === "dark";
      icon.className = "bi " + ( dark ? "bi-sun" : "bi-moon-stars" );
    };
    button.addEventListener( "click" , () => this.pick( this.find( this.current ).mode === "dark" ? "light" : "dark" ) );
    this.onChange.push( update );
    update();
  },

  // <<< feature:!appearance
  // >>> feature:appearance
  // The picker on the Appearance card: one button per preset, drawn in its
  // own colours. summary, if given, is kept showing the current theme's name.
  render( root , summary ) {
    Dom.clear( root );
    const update = () => {
      Dom.all( "button" , root ).forEach( ( button ) => {
        button.classList.toggle( "active" , button.getAttribute( "data-theme-key" ) === this.current );
        button.setAttribute( "aria-pressed" , button.getAttribute( "data-theme-key" ) === this.current ? "true" : "false" );
      } );
      if ( summary ) Dom.text( summary , I18n.get( "theme.preset_" + this.current ) );
    };
    this.presets.forEach( ( preset ) => {
      const swatch = Dom.el( "span" , { class: "theme-swatch" } );
      swatch.style.setProperty( "--swatch-bg" , this.colours( preset ).background );
      swatch.style.setProperty( "--swatch-accent" , preset.accent );
      swatch.style.setProperty( "--swatch-second" , preset.second );
      const button = Dom.el( "button" , {
        class: "btn btn-outline-secondary theme-choice",
        attrs: { type: "button" , "data-theme-key": preset.key },
        children: [ swatch , Dom.el( "span" , { text: I18n.get( "theme.preset_" + preset.key ) } ) ],
        on: { click: () => this.pick( preset.key ) },
      } );
      root.appendChild( button );
    } );
    this.onChange.push( update );
    update();
  },

  // <<< feature:appearance
  // percent of over, the rest under.
  mix( over , under , percent ) {
    const channel = function ( color , offset ) { return parseInt( color.slice( offset , offset + 2 ) , 16 ); };
    return "#" + [ 1 , 3 , 5 ].map( function ( offset ) {
      const mixed = channel( over , offset ) * percent / 100 + channel( under , offset ) * ( 100 - percent ) / 100;
      return Math.round( mixed ).toString( 16 ).padStart( 2 , "0" );
    } ).join( "" );
  },

  // Black or white, for a label on this colour.
  readableOn( color ) {
    return this.contrast( color , "#ffffff" ) >= this.contrast( color , "#0b0d10" ) ? "#ffffff" : "#0b0d10";
  },

  // This colour, moved toward black or white until text in it reads on ground.
  asText( color , ground ) {
    const away = this.luminance( ground ) > 0.4 ? "#000000" : "#ffffff";
    for ( let step = 0 ; step <= 100 ; step += 2 ) {
      const tried = this.mix( away , color , step );
      if ( this.contrast( tried , ground ) >= this.minContrast ) return tried;
    }
    return away;
  },

  // Relative luminance (WCAG) of #rrggbb.
  luminance( hex ) {
    const channel = function ( offset ) {
      const value = parseInt( hex.slice( offset , offset + 2 ) , 16 ) / 255;
      return value <= 0.03928 ? value / 12.92 : Math.pow( ( value + 0.055 ) / 1.055 , 2.4 );
    };
    return 0.2126 * channel( 1 ) + 0.7152 * channel( 3 ) + 0.0722 * channel( 5 );
  },

  contrast( first , second ) {
    const a = this.luminance( first ) , b = this.luminance( second );
    return ( Math.max( a , b ) + 0.05 ) / ( Math.min( a , b ) + 0.05 );
  },
};

Theme.restore();
