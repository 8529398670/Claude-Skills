// whole-file feature:shortcuts
package models

import (
	fmt "fmt"
	regexp "regexp"
	strings "strings"
	utf8 "unicode/utf8"

	db "{{MODULE_PATH}}/server/db"
	encryption "{{MODULE_PATH}}/server/encryption"
)

// Keyboard shortcuts are kept per account, so the keys someone picked follow
// them to every browser. Only the ones they changed are stored: an action with
// no entry uses its default, which lives in static/js/shortcuts.js with the
// action itself.
//
// A binding is OS-neutral: "Mod+Shift+Z" is Cmd+Shift+Z on a Mac and
// Ctrl+Shift+Z elsewhere, so one saved choice works on every machine. An empty
// binding switches the action's key off.
//
// The server does not know the list of actions. It checks the shape of what it
// is given, so an app that adds its own shortcut needs no change here.
const (
	MaxShortcuts = 40
)

var (
	shortcutAction = regexp.MustCompile( `^[a-z][a-z0-9_]{0,31}$` )
	// A named key, as KeyboardEvent.key spells it: Enter, Escape, ArrowUp, F5,
	// Space (the frontend's name for " ").
	shortcutNamedKey = regexp.MustCompile( `^[A-Z][A-Za-z0-9]{1,15}$` )
	shortcutModifiers = []string{ "Mod" , "Ctrl" , "Alt" , "Shift" }
)

// ValidShortcutBinding: modifiers, each at most once and in the order Mod,
// Ctrl, Alt, Shift, then one key -- a single character or a named key. The
// frontend writes bindings in exactly this form, so two spellings of the same
// keys can never both be stored.
func ValidShortcutBinding( binding string ) ( result bool ) {
	if binding == "" {
		result = true
		return
	}
	rest := binding
	next := 0
	for {
		found := false
		for index := next; index < len( shortcutModifiers ); index++ {
			prefix := shortcutModifiers[ index ] + "+"
			// "Shift++" is Shift and the + key, so a prefix only counts when
			// something follows it.
			if strings.HasPrefix( rest , prefix ) && len( rest ) > len( prefix ) {
				rest = rest[ len( prefix ): ]
				next = index + 1
				found = true
				break
			}
		}
		if found == false { break }
	}
	if utf8.RuneCountInString( rest ) == 1 {
		character , _ := utf8.DecodeRuneInString( rest )
		result = character > ' ' && character != 0x7f
		return
	}
	result = shortcutNamedKey.MatchString( rest )
	return
}

// ValidShortcuts checks a whole set before it replaces the stored one.
func ValidShortcuts( shortcuts map[string]string ) ( err error ) {
	if len( shortcuts ) > MaxShortcuts {
		err = fmt.Errorf( "at most %d shortcuts" , MaxShortcuts )
		return
	}
	// Two actions may share keys when they never apply at the same time (Esc
	// cancels an edit and closes a dialog), so clashes are the frontend's to
	// catch: it knows where each action applies.
	for action , binding := range shortcuts {
		if shortcutAction.MatchString( action ) == false {
			err = fmt.Errorf( "unknown shortcut name %q" , action )
			return
		}
		if ValidShortcutBinding( binding ) == false {
			err = fmt.Errorf( "%s: not a key the app can use" , action )
			return
		}
	}
	return
}

// SetShortcuts replaces the account's changed shortcuts. An empty map puts
// every one back to its default.
func SetShortcuts( store *db.Store , user_id uint64 , shortcuts map[string]string ) ( err error ) {
	if err = ValidShortcuts( shortcuts ); err != nil { return }
	err = store.UpdateValue( db.BucketUsers , encryption.Uint64ToBytes( user_id ) ,
		func() any { return &User{} } ,
		func( item any ) ( mutate_err error ) {
			user := item.( *User )
			if len( shortcuts ) == 0 {
				user.Shortcuts = nil
				return
			}
			user.Shortcuts = shortcuts
			return
		} )
	return
}
