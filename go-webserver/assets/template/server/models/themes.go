// whole-file feature:appearance
package models

import (
	errors "errors"
	fmt "fmt"
	regexp "regexp"
	strings "strings"
	utf8 "unicode/utf8"

	db "{{MODULE_PATH}}/server/db"
	encryption "{{MODULE_PATH}}/server/encryption"
)

// Themes are the color schemes on the Appearance screen. There are two lists:
//
//   - the app's defaults, which an admin edits. Nothing stored means the
//     built-in themes in static/js/theme.js.
//   - each person's own, on their account. Nothing stored means they follow
//     the defaults, including later changes to them; the first time they change
//     something the browser saves the whole list, and from then on it is theirs.
//
// A theme is a mode, a few colors and an optional button style, from which theme.js works out the rest
// (text, links, borders, tints) at readable contrast. Overrides replace any of
// those worked-out colors by name; dropping one puts the safe color back.
//
// A saved list also keeps which built-in themes it has seen: the highest
// "since" in theme.js's built-in list when it was saved ("builtins"). The
// browser adds any built-in theme newer than that to the list it shows, so a
// theme added to the app later reaches people with a list of their own, while
// one they deleted stays deleted.
//
// The server checks the shape only. It does not know the list of built-in
// themes, and it does not judge contrast -- an unreadable color is the
// person's own choice, and the screen warns them.
const (
	MaxThemes         = 40
	MaxThemeNameRunes = 60
	metaThemeDefaultsKey = "theme_defaults"
	metaThemeDefaultsBuiltInsKey = "theme_defaults_builtins"
	// MaxThemeBuiltIns bounds "builtins": far above any real count of built-in themes.
	MaxThemeBuiltIns = 1000
	// ThemeBuiltInsNone is the "builtins" of a list that has seen no built-in
	// theme yet, so the browser adds every one of them to it. Only
	// SetUserThemeAs saves it, for an account that followed the built-in
	// themes: the server does not know them, so it saves the one theme and
	// leaves the rest to theme.js (withNewBuiltIns). Callers cannot send it.
	ThemeBuiltInsNone = -1
)

// Theme is one of them, as stored and as the API carries it (the Theme schema
// in config/api.json; see server/apidoc for the tags).
type Theme struct {
	ID         string            `json:"id" jsonschema_description:"Lower-case letters, digits, - and _, up to 32."`
	Name       string            `json:"name" jsonschema_description:"1 to 60 characters."`
	Mode       string            `json:"mode" jsonschema:"enum=light,enum=dark"`
	Accent     string            `json:"accent" jsonschema_description:"The main color: what you can press."`
	Second     string            `json:"second" jsonschema_description:"Says where you are: active nav item, headings, info."`
	Background string            `json:"background,omitempty" jsonschema_description:"The page. Left out: the mode's own."`
	Surface    string            `json:"surface,omitempty" jsonschema_description:"Cards. Left out: the mode's own."`
	Overrides  map[string]string `json:"overrides,omitempty" jsonschema_description:"Worked-out colors replaced by name: text, accent, accentFg, accentText, second, secondFg, secondText, link, navLink, background, surface, nested, chrome, bar (the bar across the top), barText, info, infoBorder, border, rowOdd and rowEven (alternating table rows), and for each button variant V (Secondary, Success, Danger, Warning, Info, Light, Dark) btnV, btnVFg and btnVText. Leaving one out puts the readable color back."`
	Button     *ThemeButton      `json:"button,omitempty" jsonschema_description:"How the theme's buttons look. A choice left out is the default (rounded, normal, raised, normal)."`
	// The colors kept for the mode not in use (Theme.setMode in theme.js): a
	// light theme's Dark, a dark theme's Light. Switching mode swaps them in.
	Light *ThemeColors `json:"light,omitempty" jsonschema_description:"A dark theme's colors for light mode, kept while it is dark. Switching the theme to light swaps them in."`
	Dark  *ThemeColors `json:"dark,omitempty" jsonschema_description:"A light theme's colors for dark mode, kept while it is light. Switching the theme to dark swaps them in. A light theme's light (or a dark theme's dark) is dropped."`
}

// APIName and APIDoc name it in the API docs (see server/apidoc).
func ( Theme ) APIName() ( name string ) { name = "Theme"; return }

func ( Theme ) APIDoc() ( text string ) { text = "A color scheme on the Appearance screen. Colors are #rrggbb. Anything not given is worked out by static/js/theme.js at readable contrast."; return }

// ThemeColors is one mode's colors, kept on a theme while the other is in use.
type ThemeColors struct {
	Accent     string            `json:"accent" jsonschema_description:"The main color: what you can press."`
	Second     string            `json:"second" jsonschema_description:"Says where you are: active nav item, headings, info."`
	Background string            `json:"background,omitempty" jsonschema_description:"The page. Left out: the mode's own."`
	Surface    string            `json:"surface,omitempty" jsonschema_description:"Cards. Left out: the mode's own."`
	Overrides  map[string]string `json:"overrides,omitempty" jsonschema_description:"Worked-out colors replaced by name: text, accent, accentFg, accentText, second, secondFg, secondText, link, navLink, background, surface, nested, chrome, bar (the bar across the top), barText, info, infoBorder, border, rowOdd and rowEven (alternating table rows), and for each button variant V (Secondary, Success, Danger, Warning, Info, Light, Dark) btnV, btnVFg and btnVText. Leaving one out puts the readable color back."`
}

// ThemeButton is how a theme's buttons look; each choice is one of
// ThemeButtonChoices, and an empty one is the default (Bootstrap's own).
type ThemeButton struct {
	Shape  string `json:"shape,omitempty" jsonschema:"enum=square,enum=soft,enum=rounded,enum=pill"`
	Weight string `json:"weight,omitempty" jsonschema:"enum=normal,enum=medium,enum=bold"`
	Depth  string `json:"depth,omitempty" jsonschema:"enum=flat,enum=raised,enum=glossy"`
	Case   string `json:"case,omitempty" jsonschema:"enum=normal,enum=upper"`
}

var (
	themeID     = regexp.MustCompile( `^[a-z0-9_-]{1,32}$` )
	themeColor = regexp.MustCompile( `^#[0-9a-f]{6}$` )
	// The fields of Theme.colors() in theme.js that an override may replace.
	ThemeOverrideFields = map[string]bool{
		"text": true , "accent": true , "accentFg": true , "accentText": true , "second": true , "secondFg": true ,
		"secondText": true , "link": true , "navLink": true , "background": true , "surface": true ,
		"nested": true , "chrome": true , "bar": true , "barText": true , "info": true , "infoBorder": true , "border": true ,
		"rowOdd": true , "rowEven": true ,
	}
	// Theme.buttonChoices in theme.js.
	ThemeButtonChoices = map[string]map[string]bool{
		"shape": { "square": true , "soft": true , "rounded": true , "pill": true },
		"weight": { "normal": true , "medium": true , "bold": true },
		"depth": { "flat": true , "raised": true , "glossy": true },
		"case": { "normal": true , "upper": true },
	}
)

// Theme.variants in theme.js: each button variant's fill, label and outline
// can be overridden too.
func init() {
	for _ , variant := range []string{ "Secondary" , "Success" , "Danger" , "Warning" , "Info" , "Light" , "Dark" } {
		ThemeOverrideFields[ "btn" + variant ] = true
		ThemeOverrideFields[ "btn" + variant + "Fg" ] = true
		ThemeOverrideFields[ "btn" + variant + "Text" ] = true
	}
}

// cleanThemeButton checks a theme's button choices; nil when none is set.
func cleanThemeButton( id string , button *ThemeButton ) ( result *ThemeButton , err error ) {
	if button == nil { return }
	cleaned := *button
	fields := map[string]*string{ "shape": &cleaned.Shape , "weight": &cleaned.Weight , "depth": &cleaned.Depth , "case": &cleaned.Case }
	for name , value := range fields {
		*value = strings.ToLower( strings.TrimSpace( *value ) )
		if *value == "" { continue }
		if ThemeButtonChoices[ name ][ *value ] == false {
			err = fmt.Errorf( "theme %s: button %s %q is not one of the choices" , id , name , *value )
			return
		}
	}
	if cleaned != ( ThemeButton{} ) { result = &cleaned }
	return
}

// cleanThemeColors checks one mode's colors and answers them in lower case.
func cleanThemeColors( where string , colors ThemeColors ) ( result ThemeColors , err error ) {
	result = colors
	fields := map[string]*string{ "accent": &result.Accent , "second": &result.Second , "background": &result.Background , "surface": &result.Surface }
	for name , color := range fields {
		*color = strings.ToLower( *color )
		optional := name == "background" || name == "surface"
		if optional && *color == "" { continue }
		if themeColor.MatchString( *color ) == false {
			err = fmt.Errorf( "%s: %s is not a #rrggbb color" , where , name )
			return
		}
	}
	overrides := map[string]string{}
	for field , color := range colors.Overrides {
		color = strings.ToLower( color )
		if ThemeOverrideFields[ field ] == false {
			err = fmt.Errorf( "%s: no color called %q" , where , field )
			return
		}
		if themeColor.MatchString( color ) == false {
			err = fmt.Errorf( "%s: %s is not a #rrggbb color" , where , field )
			return
		}
		overrides[ field ] = color
	}
	result.Overrides = nil
	if len( overrides ) > 0 { result.Overrides = overrides }
	return
}

func ValidThemeID( id string ) ( result bool ) {
	result = themeID.MatchString( id )
	return
}

// CleanThemes checks a whole list before it replaces a stored one, and answers
// it tidied: names trimmed, colors in lower case.
func CleanThemes( themes []Theme ) ( result []Theme , err error ) {
	if len( themes ) == 0 {
		err = errors.New( "at least one theme" )
		return
	}
	if len( themes ) > MaxThemes {
		err = fmt.Errorf( "at most %d themes" , MaxThemes )
		return
	}
	seen := map[string]bool{}
	result = make( []Theme , 0 , len( themes ) )
	for _ , theme := range themes {
		if ValidThemeID( theme.ID ) == false {
			err = fmt.Errorf( "theme id %q: letters, digits, - and _, up to 32" , theme.ID )
			return
		}
		if seen[ theme.ID ] {
			err = fmt.Errorf( "theme id %q used twice" , theme.ID )
			return
		}
		seen[ theme.ID ] = true
		theme.Name = strings.TrimSpace( theme.Name )
		if theme.Name == "" || utf8.RuneCountInString( theme.Name ) > MaxThemeNameRunes {
			err = fmt.Errorf( "theme %s: a name of 1 to %d characters" , theme.ID , MaxThemeNameRunes )
			return
		}
		if theme.Mode != "light" && theme.Mode != "dark" {
			err = fmt.Errorf( "theme %s: mode is light or dark" , theme.ID )
			return
		}
		colors := ThemeColors{ Accent: theme.Accent , Second: theme.Second , Background: theme.Background , Surface: theme.Surface , Overrides: theme.Overrides }
		if colors , err = cleanThemeColors( "theme " + theme.ID , colors ); err != nil { return }
		theme.Accent , theme.Second , theme.Background , theme.Surface , theme.Overrides = colors.Accent , colors.Second , colors.Background , colors.Surface , colors.Overrides
		// Only the mode not in use is kept; one for the mode in use means nothing.
		if theme.Mode == "light" { theme.Light = nil } else { theme.Dark = nil }
		for mode , kept := range map[string]**ThemeColors{ "light": &theme.Light , "dark": &theme.Dark } {
			if *kept == nil { continue }
			var cleaned ThemeColors
			if cleaned , err = cleanThemeColors( "theme " + theme.ID + " " + mode , **kept ); err != nil { return }
			*kept = &cleaned
		}
		if theme.Button , err = cleanThemeButton( theme.ID , theme.Button ); err != nil { return }
		result = append( result , theme )
	}
	return
}

// CleanThemeBuiltIns checks a list's "builtins": 0 to MaxThemeBuiltIns.
func CleanThemeBuiltIns( builtins int ) ( err error ) {
	if builtins < 0 || builtins > MaxThemeBuiltIns {
		err = fmt.Errorf( "builtins is 0 to %d" , MaxThemeBuiltIns )
	}
	return
}

// ThemeDefaults is the admin's list, or nil when none has been saved and the
// built-in themes apply; and which built-in themes it has seen.
func ThemeDefaults( store *db.Store ) ( themes []Theme , builtins int , err error ) {
	err = store.Get( db.BucketMeta , []byte( metaThemeDefaultsKey ) , &themes )
	if errors.Is( err , db.ErrNotFound ) {
		themes = nil
		err = nil
		return
	}
	if err != nil { return }
	// A list saved before builtins existed has none: 0, the first built-ins.
	err = store.Get( db.BucketMeta , []byte( metaThemeDefaultsBuiltInsKey ) , &builtins )
	if errors.Is( err , db.ErrNotFound ) { err = nil }
	return
}

// ErrThemesFull: SetUserThemeAs would add a theme to a list already at MaxThemes.
var ErrThemesFull = fmt.Errorf( "the account already has %d themes, the most it can have" , MaxThemes )

// themeDefaultsTx is ThemeDefaults inside the caller's transaction.
func themeDefaultsTx( tx *db.Tx ) ( themes []Theme , builtins int , err error ) {
	err = tx.Get( db.BucketMeta , []byte( metaThemeDefaultsKey ) , &themes )
	if errors.Is( err , db.ErrNotFound ) {
		themes = nil
		err = nil
		return
	}
	if err != nil { return }
	err = tx.Get( db.BucketMeta , []byte( metaThemeDefaultsBuiltInsKey ) , &builtins )
	if errors.Is( err , db.ErrNotFound ) { err = nil }
	return
}

// SetThemeDefaults replaces the admin's list and which built-in themes it has
// seen. nil goes back to the built-in themes.
func SetThemeDefaults( store *db.Store , themes []Theme , builtins int ) ( err error ) {
	if themes == nil {
		if err = store.Delete( db.BucketMeta , []byte( metaThemeDefaultsKey ) ); err != nil { return }
		err = store.Delete( db.BucketMeta , []byte( metaThemeDefaultsBuiltInsKey ) )
		return
	}
	if themes , err = CleanThemes( themes ); err != nil { return }
	if err = CleanThemeBuiltIns( builtins ); err != nil { return }
	if err = store.Put( db.BucketMeta , []byte( metaThemeDefaultsKey ) , themes ); err != nil { return }
	err = store.Put( db.BucketMeta , []byte( metaThemeDefaultsBuiltInsKey ) , builtins )
	return
}

// SetUserThemes replaces someone's own list, which built-in themes it has
// seen, and their pick. nil themes go back to following the defaults.
func SetUserThemes( store *db.Store , user_id uint64 , themes []Theme , builtins int , theme_id string ) ( err error ) {
	if themes != nil {
		if themes , err = CleanThemes( themes ); err != nil { return }
		if err = CleanThemeBuiltIns( builtins ); err != nil { return }
	} else {
		builtins = 0
	}
	if theme_id != "" && ValidThemeID( theme_id ) == false {
		err = errors.New( "not a theme id" )
		return
	}
	err = store.UpdateValue( db.BucketUsers , encryption.Uint64ToBytes( user_id ) ,
		func() any { return &User{} } ,
		func( item any ) ( mutate_err error ) {
			user := item.( *User )
			user.Themes = themes
			user.ThemesBuiltIns = builtins
			user.ThemeID = theme_id
			return
		} )
	return
}

// SetUserThemeID changes only which theme someone uses. The id is not checked
// against their list: a list that follows the defaults can change under it,
// and the browser falls back to the first theme for an id it cannot find.
func SetUserThemeID( store *db.Store , user_id uint64 , theme_id string ) ( err error ) {
	if theme_id != "" && ValidThemeID( theme_id ) == false {
		err = errors.New( "not a theme id" )
		return
	}
	err = store.UpdateValue( db.BucketUsers , encryption.Uint64ToBytes( user_id ) ,
		func() any { return &User{} } ,
		func( item any ) ( mutate_err error ) {
			item.( *User ).ThemeID = theme_id
			return
		} )
	return
}

// SetUserThemeAs is an admin (ProjectPortal's shared theme, portal contract
// capability user_theme) putting one theme on someone's account and making it
// their pick. It starts from the list they see today -- their own, else the
// admin's defaults, else the built-in themes -- replaces the theme with the
// same id or adds it at the end, and saves that as their own list. The
// built-in themes are not known here, so for an account that followed them the
// saved list is the one theme with ThemeBuiltInsNone, and the browser adds
// every built-in theme to it. Held to mayActOnTx; a missing account is
// db.ErrNotFound, a full list ErrThemesFull, and a bad theme CleanThemes' error.
func SetUserThemeAs( store *db.Store , actor_id uint64 , user_id uint64 , theme Theme ) ( theme_id string , err error ) {
	cleaned , err := CleanThemes( []Theme{ theme } )
	if err != nil { return }
	theme = cleaned[ 0 ]
	err = store.Write( func( tx *db.Tx ) ( tx_err error ) {
		key := encryption.Uint64ToBytes( user_id )
		user := &User{}
		if tx_err = tx.Get( db.BucketUsers , key , user ); tx_err != nil { return }
		if tx_err = mayActOnTx( tx , actor_id , user ); tx_err != nil { return }
		list , builtins := user.Themes , user.ThemesBuiltIns
		if list == nil {
			if list , builtins , tx_err = themeDefaultsTx( tx ); tx_err != nil { return }
		}
		if list == nil { builtins = ThemeBuiltInsNone }
		merged := make( []Theme , 0 , len( list ) + 1 )
		replaced := false
		for _ , each := range list {
			if each.ID == theme.ID {
				each = theme
				replaced = true
			}
			merged = append( merged , each )
		}
		if replaced == false {
			if len( merged ) >= MaxThemes {
				tx_err = ErrThemesFull
				return
			}
			merged = append( merged , theme )
		}
		user.Themes = merged
		user.ThemesBuiltIns = builtins
		user.ThemeID = theme.ID
		tx_err = tx.Put( db.BucketUsers , key , user )
		return
	} )
	if err == nil { theme_id = theme.ID }
	return
}
