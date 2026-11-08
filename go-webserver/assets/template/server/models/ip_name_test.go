// whole-file feature:logs
package models

import (
	testing "testing"
)

func TestCleanIPTargetKeepsOneSpelling( t *testing.T ) {
	cases := map[ string ]string{
		"10.0.0.5":           "10.0.0.5",
		" 10.0.0.5 ":         "10.0.0.5",
		"::ffff:10.0.0.5":    "10.0.0.5",
		"10.0.0.77/24":       "10.0.0.0/24",
		"10.0.0.5/32":        "10.0.0.5",
		"2001:DB8::1":        "2001:db8::1",
		"2001:db8::1/48":     "2001:db8::/48",
	}
	for text , want := range cases {
		if got , ok := CleanIPTarget( text ); ok == false || got != want {
			t.Fatalf( "%q: got %q %v, want %q" , text , got , ok , want )
		}
	}
	for _ , text := range []string{ "" , "laptop" , "10.0.0" , "10.0.0.0/33" } {
		if got , ok := CleanIPTarget( text ); ok {
			t.Fatalf( "%q was taken as %q" , text , got )
		}
	}
}

func TestTheMostSpecificNameWins( t *testing.T ) {
	store := testStore( t )
	for target , name := range map[ string ]string{
		"10.0.0.0/8": "Company" , "10.1.0.0/16": "Office" , "10.1.2.3": "Printer" , "2001:db8::/32": "Six" ,
	} {
		if err := SetIPName( store , target , name ); err != nil { t.Fatal( err ) }
	}
	namer , err := LoadIPNamer( store )
	if err != nil { t.Fatal( err ) }
	cases := map[ string ]string{
		"10.1.2.3": "Printer" , "::ffff:10.1.2.3": "Printer" , "10.1.2.4": "Office" , "10.9.9.9": "Company" ,
		"2001:db8::9": "Six" , "11.0.0.1": "" , "not an address": "" , "": "" ,
	}
	for ip , want := range cases {
		for range 2 {
			if got := namer.Name( ip ); got != want { t.Fatalf( "%q: got %q, want %q" , ip , got , want ) }
		}
	}
}

func TestANameCanBeChangedAndTakenAway( t *testing.T ) {
	store := testStore( t )
	if err := SetIPName( store , "10.0.0.5" , "  Old  " ); err != nil { t.Fatal( err ) }
	if err := SetIPName( store , "10.0.0.5" , "New" ); err != nil { t.Fatal( err ) }
	names , err := ListIPNames( store )
	if err != nil || len( names ) != 1 || names[ 0 ].Name != "New" {
		t.Fatalf( "%v %v" , names , err )
	}
	if err = DeleteIPName( store , "10.0.0.5" ); err != nil { t.Fatal( err ) }
	if names , _ = ListIPNames( store ); len( names ) != 0 { t.Fatalf( "%v" , names ) }
}
