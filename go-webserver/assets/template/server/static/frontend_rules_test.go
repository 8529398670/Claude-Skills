package static

import (
	os "os"
	filepath "path/filepath"
	regexp "regexp"
	strings "strings"
	testing "testing"
)

// The hard rules for the frontend (see "Hard rules" in
// references/frontend.md), the parts a test can see:
//
//   - Mobile first: media queries add from a breakpoint up (min-width). The one
//     max-width query allowed is a phone-only form of a component, and says so
//     in the comment right above it with the word "phone-only".
//   - Layout is Bootstrap's grid: no grid-template-columns, except inside the
//     few components Bootstrap has no class for.
//   - Every table scrolls in its own .table-responsive box, so a wide one never
//     makes the page scroll sideways on a phone.
//
// How much room a table gets and which icons are used are judgement calls with
// good-reason exceptions, so they are written rules and not tested here.

const frontendRoot = "../../static"

// Components that are a grid inside themselves and have no Bootstrap class:
// the calendar's seven days, the small picture of a theme, and the ? list of
// key caps beside what they do.
var gridComponents = map[ string ]bool{
	".log-cal-grid": true,
	".theme-preview": true,
	".shortcut-list": true,
}

var cssComment = regexp.MustCompile( `(?s)/\*.*?\*/` )

func readFrontend( t *testing.T , name string ) string {
	t.Helper()
	data , err := os.ReadFile( filepath.Join( frontendRoot , name ) )
	if err != nil { t.Fatalf( "read %s: %v" , name , err ) }
	return string( data )
}

func frontendScripts( t *testing.T ) map[ string ]string {
	t.Helper()
	names , err := filepath.Glob( filepath.Join( frontendRoot , "js" , "*.js" ) )
	if err != nil { t.Fatal( err ) }
	scripts := map[ string ]string{}
	for _ , name := range names {
		data , err := os.ReadFile( name )
		if err != nil { t.Fatal( err ) }
		scripts[ filepath.Base( name ) ] = string( data )
	}
	return scripts
}

func TestFrontendIsMobileFirst( t *testing.T ) {
	css := readFrontend( t , "css/style.css" )
	media := regexp.MustCompile( `@media[^{]*` )
	for _ , at := range media.FindAllStringIndex( css , -1 ) {
		query := css[ at[ 0 ]:at[ 1 ] ]
		if !strings.Contains( query , "max-width" ) { continue }
		before := strings.TrimRight( css[ :at[ 0 ] ] , " \t\r\n" )
		comment := ""
		if strings.HasSuffix( before , "*/" ) {
			if start := strings.LastIndex( before , "/*" ); start >= 0 { comment = before[ start: ] }
		}
		if !strings.Contains( comment , "phone-only" ) {
			t.Errorf( "style.css: %q is desktop first. Write the phone as the base and add from a breakpoint up with min-width (frontend.md, Hard rules)" , strings.TrimSpace( query ) )
		}
	}
}

func TestFrontendLayoutIsBootstrapGrid( t *testing.T ) {
	css := cssComment.ReplaceAllString( readFrontend( t , "css/style.css" ) , "" )
	rule := regexp.MustCompile( `([^{}]+)\{([^{}]*)\}` )
	for _ , match := range rule.FindAllStringSubmatch( css , -1 ) {
		if !strings.Contains( match[ 2 ] , "grid-template-columns" ) { continue }
		for _ , selector := range strings.Split( match[ 1 ] , "," ) {
			selector = strings.TrimSpace( selector )
			if !gridComponents[ selector ] {
				t.Errorf( "style.css: %s lays out with grid-template-columns. Use Bootstrap's .row / .col-* / row-cols-* (frontend.md, Hard rules)" , selector )
			}
		}
	}
	sources := frontendScripts( t )
	sources[ "index.html" ] = readFrontend( t , "index.html" )
	for name , text := range sources {
		if strings.Contains( text , "grid-template-columns" ) || strings.Contains( text , "gridTemplateColumns" ) {
			t.Errorf( "%s sets grid columns itself. Use Bootstrap's .row / .col-* (frontend.md, Hard rules)" , name )
		}
	}
}

func TestEveryTableScrollsInItsOwnBox( t *testing.T ) {
	html := readFrontend( t , "index.html" )
	openTag := regexp.MustCompile( `<[a-zA-Z][^>]*>` )
	table := regexp.MustCompile( `<table\b` )
	for _ , at := range table.FindAllStringIndex( html , -1 ) {
		tags := openTag.FindAllString( html[ :at[ 0 ] ] , -1 )
		parent := ""
		if len( tags ) > 0 { parent = tags[ len( tags ) - 1 ] }
		if !strings.Contains( parent , "table-responsive" ) {
			line := strings.Count( html[ :at[ 0 ] ] , "\n" ) + 1
			t.Errorf( "index.html:%d: put this table right inside a <div class=\"table-responsive\"> (frontend.md, Hard rules)" , line )
		}
	}
	for name , text := range frontendScripts( t ) {
		if strings.Contains( text , `Dom.el( "table"` ) && !strings.Contains( text , `"table-responsive"` ) {
			t.Errorf( "%s builds a table without a .table-responsive box around it (frontend.md, Hard rules)" , name )
		}
	}
}
