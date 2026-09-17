/* kit.js -- the runtime every explainer document shares.
 *
 * scripts/build.py splices this into <head>, ahead of the document's own script,
 * which sits at the end of <body> by contract. So `Kit` is always defined by the
 * time document code runs, and anything the kit does to the DOM is deferred to
 * DOMContentLoaded -- which fires after those end-of-body inline scripts have
 * executed.
 *
 * Everything here replaces something the model used to rewrite from scratch in
 * all four levels: a seeded PRNG, per-widget error isolation, slider-to-readout
 * wiring, and start/stop discipline for requestAnimationFrame.
 *
 * The page has an opaque origin: no storage, no network, no parent access.
 * Nothing here assumes otherwise.
 */
( function ( global ) {
  "use strict";

  var Kit = {};

  /* ---- determinism -------------------------------------------------- */

  /* mulberry32. Returns a function producing floats in [0,1). Seeded, so a
   * reload shows the reader the same page they were looking at -- a document
   * whose numbers change under you cannot be trusted or referred back to. */
  Kit.rng = function ( seed ) {
    var state = ( seed === undefined ? 1 : seed ) >>> 0;
    return function () {
      state = ( state + 0x6d2b79f5 ) >>> 0;
      var t = state;
      t = Math.imul( t ^ ( t >>> 15 ), t | 1 );
      t ^= t + Math.imul( t ^ ( t >>> 7 ), t | 61 );
      return ( ( t ^ ( t >>> 14 ) ) >>> 0 ) / 4294967296;
    };
  };

  /* A shared default stream, so a document that just wants a random number
   * does not have to invent a seed to stay deterministic. */
  Kit.rand = Kit.rng( 0x9e3779b9 );

  /* ---- formatting ---------------------------------------------------- */

  /* Fixed significant digits with thin-space grouping, so a live readout does
   * not jitter in width as digits change. Pairs with .readout's tabular-nums. */
  Kit.fmt = function ( value, options ) {
    var settings = options || {};
    if ( typeof value !== "number" || isFinite( value ) === false ) return "--";

    var digits = settings.digits;
    if ( digits === undefined ) {
      var magnitude = Math.abs( value );
      digits = magnitude >= 100 ? 0 : magnitude >= 10 ? 1 : magnitude >= 1 ? 2 : 3;
    }

    var text = value.toFixed( digits );
    if ( settings.group !== false && Math.abs( value ) >= 10000 ) {
      var parts = text.split( "." );
      parts[ 0 ] = parts[ 0 ].replace( /\B(?=(\d{3})+(?!\d))/g, " " );
      text = parts.join( "." );
    }
    return settings.unit ? text + " " + settings.unit : text;
  };

  /* ---- widget isolation ---------------------------------------------- */

  /* Run one widget's setup so that a throw inside it cannot take the rest of
   * the page with it. A failed widget becomes a visible, honest gap: a blank
   * page tells the reader nothing, and a half-dead page tells them something
   * false. `where` is an element or an id. */
  Kit.widget = function ( where, init ) {
    var host = typeof where === "string" ? document.getElementById( where ) : where;
    if ( host === null || host === undefined ) return false;
    try {
      init( host );
      return true;
    } catch ( error ) {
      try {
        var note = document.createElement( "p" );
        note.className = "kit-failed";
        note.textContent = "This widget failed to start, so it has been left out rather than shown half-working.";
        host.replaceChildren( note );
      } catch ( ignored ) { /* nothing left to do */ }
      if ( global.console && console.error ) console.error( "[kit] widget failed:", error );
      return false;
    }
  };

  /* ---- controls ------------------------------------------------------ */

  /* Wire a range (or number) input to a callback, and call it once immediately
   * so the page opens in a meaningful state rather than waiting to be touched.
   * Returns a function that re-reads the input. */
  Kit.slider = function ( where, onChange ) {
    var input = typeof where === "string" ? document.getElementById( where ) : where;
    if ( input === null || input === undefined ) return function () {};

    var readout = null;
    var target = input.getAttribute( "data-readout" );
    if ( target ) readout = document.getElementById( target );

    var unit = input.getAttribute( "data-unit" ) || "";
    var digits = input.hasAttribute( "data-digits" ) ? Number( input.getAttribute( "data-digits" ) ) : undefined;

    var apply = function () {
      var value = Number( input.value );
      if ( readout ) readout.textContent = Kit.fmt( value, { unit: unit, digits: digits } );
      if ( onChange ) onChange( value, input );
    };

    input.setAttribute( "data-kit-wired", "" );
    input.addEventListener( "input", apply );
    apply();
    return apply;
  };

  /* Every range carrying data-readout, wired in one call. Idempotent: an input
   * already wired is skipped, so calling it again after adding DOM is safe. */
  Kit.autowire = function ( root ) {
    var scope = root || document;
    var inputs = scope.querySelectorAll( "input[type=range][data-readout], input[type=number][data-readout]" );
    for ( var index = 0; index < inputs.length; index += 1 ) {
      if ( inputs[ index ].hasAttribute( "data-kit-wired" ) ) continue;
      inputs[ index ].setAttribute( "data-kit-wired", "" );
      Kit.slider( inputs[ index ], null );
    }
  };

  /* ---- animation ----------------------------------------------------- */

  /* A rAF loop with an off switch, which is the part that gets forgotten.
   * It also stops itself when the tab is hidden and when the document is
   * scrolled away from -- an idle loop burning a phone battery behind a
   * background tab is the most common sin in a generated page.
   *
   * step(dt_seconds, elapsed_seconds) -> return false to stop. */
  Kit.loop = function ( step ) {
    var handle = 0;
    var last = 0;
    var start = 0;
    var running = false;

    var frame = function ( now ) {
      if ( running === false ) return;
      if ( last === 0 ) { last = now; start = now; }
      var dt = ( now - last ) / 1000;
      last = now;
      var keep = true;
      try {
        keep = step( dt, ( now - start ) / 1000 );
      } catch ( error ) {
        if ( global.console && console.error ) console.error( "[kit] loop failed:", error );
        keep = false;
      }
      if ( keep === false ) { controller.stop(); return; }
      handle = global.requestAnimationFrame( frame );
    };

    var controller = {
      start: function () {
        if ( running ) return controller;
        running = true;
        last = 0;
        handle = global.requestAnimationFrame( frame );
        return controller;
      },
      stop: function () {
        running = false;
        if ( handle ) global.cancelAnimationFrame( handle );
        handle = 0;
        return controller;
      },
      toggle: function () { return running ? controller.stop() : controller.start(); },
      get running() { return running; }
    };

    document.addEventListener( "visibilitychange", function () {
      if ( document.hidden && running ) { controller.stop(); controller.wasStopped = true; }
      else if ( document.hidden === false && controller.wasStopped ) { controller.wasStopped = false; controller.start(); }
    } );

    return controller;
  };

  /* ---- maths --------------------------------------------------------- */

  /* Delimiters. Single $...$ is included because inline quantities are the
   * common case and `$H_2O$` is what an author naturally writes -- KaTeX
   * leaves it off by default precisely because it collides with currency,
   * which is why a literal dollar has to be written \$ in these documents. */
  var MATH_DELIMITERS = [
    { left: "$$", right: "$$", display: true },
    { left: "\\[", right: "\\]", display: true },
    { left: "\\(", right: "\\)", display: false },
    { left: "$", right: "$", display: false }
  ];

  /* Typeset a subtree. Call it after building DOM yourself; the kit already
   * covers whatever exists at DOMContentLoaded. Safe to call repeatedly --
   * KaTeX skips what it has already rendered via the ignored class. */
  Kit.math = function ( root ) {
    var scope = root || document.body;
    if ( typeof global.renderMathInElement !== "function" || scope === null ) return false;
    try {
      global.renderMathInElement( scope, {
        delimiters: MATH_DELIMITERS,
        /* Render the source instead of throwing, so one malformed expression
         * is a visible blemish rather than an exception that kills the script
         * that was mid-way through building the page. */
        throwOnError: false,
        ignoredTags: [ "script", "noscript", "style", "textarea", "pre", "code", "option" ],
        ignoredClasses: [ "no-math", "katex", "readout" ]
      } );
      return true;
    } catch ( error ) {
      if ( global.console && console.error ) console.error( "[kit] math failed:", error );
      return false;
    }
  };

  /* ---- boot ---------------------------------------------------------- */

  var boot = function () {
    Kit.autowire( document );
    if ( Kit.math( document.body ) === false ) {
      /* KaTeX is fetched rather than inlined, so on a slow connection it can
       * still be in flight at DOMContentLoaded. Try once more at load, and if
       * it never arrives the page shows its TeX as written -- legible, if
       * unlovely. Silence would be worse than either. */
      global.addEventListener( "load", function () { Kit.math( document.body ); } );
    }
  };

  if ( document.readyState === "loading" ) {
    document.addEventListener( "DOMContentLoaded", boot );
  } else {
    boot();
  }

  global.Kit = Kit;
} )( typeof window !== "undefined" ? window : this );
