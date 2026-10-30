// The sidebar: which signed-in screen is showing.
//
// Every screen is a <section class="view" data-view="name"> in index.html and
// a sidebar link to #name. The URL hash is the only state -- back, forward,
// reload and a pasted link all land on the same screen, with no router.
//
// Whether a screen exists for this user is not decided here. The screen's own
// file un-hides its section (Keys.init shows the API keys panel, and only if
// the server has them switched on); Nav only offers what was un-hidden. So
// app.js calls Nav.init() last, once every screen has had its say.
const Nav = {
  fallback: "account",

  init() {
    Dom.all( "[data-nav]" ).forEach( ( link ) => {
      Dom.show( link , this.available( link.getAttribute( "data-nav" ) ) );
      link.addEventListener( "click" , () => this.closeSidebar() );
    } );
    // A group heading with nothing left under it goes too.
    Dom.all( ".sidebar-group" ).forEach( function ( group ) {
      Dom.show( group , Dom.all( "[data-nav]:not([hidden])" , group ).length > 0 );
    } );
    Dom.show( Dom.get( "sidebar" ) , true );
    Dom.show( Dom.get( "sidebar-toggle" ) , true );

    window.addEventListener( "hashchange" , () => this.render() );
    this.render();
  },

  available( name ) {
    const view = document.querySelector( '.view[data-view="' + name + '"]' );
    return Boolean( view ) && !view.hidden;
  },

  // A hash naming a screen this user does not have (an admin link opened by a
  // non-admin, a typo) shows the account screen rather than nothing.
  current() {
    const name = window.location.hash.replace( /^#/ , "" );
    return this.available( name ) ? name : this.fallback;
  },

  render() {
    const name = this.current();
    Dom.all( ".view" ).forEach( function ( view ) {
      view.classList.toggle( "view-active" , view.getAttribute( "data-view" ) === name );
    } );
    Dom.all( "[data-nav]" ).forEach( function ( link ) {
      const active = link.getAttribute( "data-nav" ) === name;
      link.classList.toggle( "active" , active );
      if ( active ) link.setAttribute( "aria-current" , "page" );
      else link.removeAttribute( "aria-current" );
    } );
    window.scrollTo( 0 , 0 );
  },

  // On a phone the sidebar is a slide-in panel; picking a screen closes it,
  // including the one already showing (which changes no hash).
  closeSidebar() {
    const panel = window.bootstrap && bootstrap.Offcanvas.getInstance( Dom.get( "sidebar" ) );
    if ( panel ) panel.hide();
  },
};
