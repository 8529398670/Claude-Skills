#!/usr/bin/env node
// Holds every theme in static/js/theme.js to a readable contrast: the same maths
// the page runs, so a colour added to a preset cannot quietly fail. Run by the
// tests, or on its own: node scripts/check_themes.js
const fs = require( "fs" );
const path = require( "path" );
const vm = require( "vm" );

const source = fs.readFileSync( path.join( __dirname , ".." , "static" , "js" , "theme.js" ) , "utf8" );
const page = {
  document: { documentElement: { setAttribute() {} , removeAttribute() {} , style: { setProperty() {} } } },
  window: { localStorage: { getItem() { return ""; } } },
};
vm.createContext( page );
vm.runInContext( source + "\nglobalThis.Theme = Theme;" , page );
const Theme = page.Theme;

let failed = 0;
Theme.presets.forEach( ( preset ) => {
  const c = Theme.colours( preset );
  const pairs = {
    "text on a card": [ c.text , c.surface ],
    "text on a nested card": [ c.text , c.nested ],
    "text in an info box": [ c.text , c.info ],
    "link on a nested card": [ c.link , c.nested ],
    "button label": [ c.accentFg , c.accent ],
    "heading on a nested card": [ c.secondText , c.nested ],
    "active nav label": [ c.secondFg , c.second ],
    "sidebar link": [ c.navLink , c.chrome ],
  };
  Object.entries( pairs ).forEach( ( [ name , [ front , back ] ] ) => {
    const ratio = Theme.contrast( front , back );
    if ( ratio >= Theme.minContrast ) return;
    failed += 1;
    console.error( preset.key + ": " + name + " is " + ratio.toFixed( 2 ) + " (" + front + " on " + back + ")" );
  } );
} );
if ( failed > 0 ) process.exit( 1 );
console.log( "ok: " + Theme.presets.length + " themes" );
