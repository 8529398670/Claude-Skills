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

// Themes are the colour schemes on the Appearance screen. There are two lists:
//
//   - the app's defaults, which an admin edits. Nothing stored means the
//     built-in themes in static/js/theme.js.
//   - each person's own, on their account. Nothing stored means they follow
//     the defaults, including later changes to them; the first time they change
//     something the browser saves the whole list, and from then on it is theirs.
//
// A theme is a mode and a few colours, from which theme.js works out the rest
// (text, links, borders, tints) at readable contrast. Overrides replace any of
// those worked-out colours by name; dropping one puts the safe colour back.
//
// The server checks the shape only. It does not know the list of built-in
// themes, and it does not judge contrast -- an unreadable colour is the
// person's own choice, and the screen warns them.
const (
	MaxThemes         = 40
	MaxThemeNameRunes = 60
	metaThemeDefaultsKey = "theme_defaults"
)

type Theme struct {
	ID         string            `json:"id"`
	Name       string            `json:"name"`
	Mode       string            `json:"mode"`
	Accent     string            `json:"accent"`
	Second     string            `json:"second"`
	Background string            `json:"background,omitempty"`
	Surface    string            `json:"surface,omitempty"`
	Overrides  map[string]string `json:"overrides,omitempty"`
}

var (
	themeID     = regexp.MustCompile( `^[a-z0-9_-]{1,32}$` )
	themeColour = regexp.MustCompile( `^#[0-9a-f]{6}$` )
	// The fields of Theme.colours() in theme.js that an override may replace.
	ThemeOverrideFields = map[string]bool{
		"text": true , "accent": true , "accentFg": true , "second": true , "secondFg": true ,
		"secondText": true , "link": true , "navLink": true , "background": true , "surface": true ,
		"nested": true , "chrome": true , "info": true , "infoBorder": true , "border": true ,
	}
)

func ValidThemeID( id string ) ( result bool ) {
	result = themeID.MatchString( id )
	return
}

// CleanThemes checks a whole list before it replaces a stored one, and answers
// it tidied: names trimmed, colours in lower case.
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
		colours := map[string]*string{ "accent": &theme.Accent , "second": &theme.Second , "background": &theme.Background , "surface": &theme.Surface }
		for name , colour := range colours {
			*colour = strings.ToLower( *colour )
			optional := name == "background" || name == "surface"
			if optional && *colour == "" { continue }
			if themeColour.MatchString( *colour ) == false {
				err = fmt.Errorf( "theme %s: %s is not a #rrggbb colour" , theme.ID , name )
				return
			}
		}
		overrides := map[string]string{}
		for field , colour := range theme.Overrides {
			colour = strings.ToLower( colour )
			if ThemeOverrideFields[ field ] == false {
				err = fmt.Errorf( "theme %s: no colour called %q" , theme.ID , field )
				return
			}
			if themeColour.MatchString( colour ) == false {
				err = fmt.Errorf( "theme %s: %s is not a #rrggbb colour" , theme.ID , field )
				return
			}
			overrides[ field ] = colour
		}
		theme.Overrides = nil
		if len( overrides ) > 0 { theme.Overrides = overrides }
		result = append( result , theme )
	}
	return
}

// ThemeDefaults is the admin's list, or nil when none has been saved and the
// built-in themes apply.
func ThemeDefaults( store *db.Store ) ( themes []Theme , err error ) {
	err = store.Get( db.BucketMeta , []byte( metaThemeDefaultsKey ) , &themes )
	if errors.Is( err , db.ErrNotFound ) {
		themes = nil
		err = nil
	}
	return
}

// SetThemeDefaults replaces the admin's list. nil goes back to the built-in
// themes.
func SetThemeDefaults( store *db.Store , themes []Theme ) ( err error ) {
	if themes == nil {
		err = store.Delete( db.BucketMeta , []byte( metaThemeDefaultsKey ) )
		return
	}
	if themes , err = CleanThemes( themes ); err != nil { return }
	err = store.Put( db.BucketMeta , []byte( metaThemeDefaultsKey ) , themes )
	return
}

// SetUserThemes replaces someone's own list and pick. nil themes go back to
// following the defaults.
func SetUserThemes( store *db.Store , user_id uint64 , themes []Theme , theme_id string ) ( err error ) {
	if themes != nil {
		if themes , err = CleanThemes( themes ); err != nil { return }
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
