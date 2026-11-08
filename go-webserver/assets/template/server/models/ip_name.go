// whole-file feature:logs
package models

import (
	netip "net/netip"
	sort "sort"
	strings "strings"
	time "time"
	utf8 "unicode/utf8"

	db "{{MODULE_PATH}}/server/db"
)

const ipNameMax = 80

// IPName is what the owner calls an address or a range of them, so the Logs
// and Sessions screens can say who is who. It is applied when something reads
// the log: the log itself keeps the address, so renaming one renames its past
// too. Target is the key, in the one spelling CleanIPTarget gives it.
type IPName struct {
	Target    string    `json:"target"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"created_at"`
}

// CleanIPTarget reads an address (10.0.0.5) or a range (10.0.0.0/24), and says
// it the one way it is stored: a range starts at its first address, and a range
// of exactly one address is that address.
func CleanIPTarget( text string ) ( target string , ok bool ) {
	text = strings.TrimSpace( text )
	if address , err := netip.ParseAddr( text ); err == nil {
		target = address.Unmap().WithZone( "" ).String()
		ok = true
		return
	}
	prefix , err := netip.ParsePrefix( text )
	if err != nil { return }
	prefix = prefix.Masked()
	if prefix.IsSingleIP() {
		target = prefix.Addr().String()
	} else {
		target = prefix.String()
	}
	ok = true
	return
}

func ValidIPName( name string ) ( result bool ) {
	length := utf8.RuneCountInString( strings.TrimSpace( name ) )
	result = length >= 1 && length <= ipNameMax
	return
}

// SetIPName names a target, or renames it.
func SetIPName( store *db.Store , target string , name string ) ( err error ) {
	record := &IPName{ Target: target , Name: strings.TrimSpace( name ) , CreatedAt: time.Now().UTC() }
	err = store.Put( db.BucketIPNames , []byte( target ) , record )
	return
}

func DeleteIPName( store *db.Store , target string ) ( err error ) {
	err = store.Delete( db.BucketIPNames , []byte( target ) )
	return
}

// ListIPNames is every name, by what it is called.
func ListIPNames( store *db.Store ) ( names []*IPName , err error ) {
	names = []*IPName{}
	err = store.ForEach( db.BucketIPNames ,
		func() any { return &IPName{} } ,
		func( key []byte , item any ) bool {
			names = append( names , item.( *IPName ) )
			return true
		} )
	sort.SliceStable( names , func( i int , j int ) bool {
		return strings.ToLower( names[ i ].Name ) < strings.ToLower( names[ j ].Name )
	} )
	return
}

// IPNamer answers what an address is called, for one read of the log, which can
// ask about the same few addresses thousands of times. It is not for sharing
// between requests: it remembers its answers without a lock.
type IPNamer struct {
	exact  map[ string ]string
	ranges []namedRange
	seen   map[ string ]string
}

type namedRange struct {
	prefix netip.Prefix
	name   string
}

// LoadIPNamer reads the names once. The most specific one wins: an exact address
// over any range, and a narrower range over a wider one.
func LoadIPNamer( store *db.Store ) ( namer *IPNamer , err error ) {
	names , err := ListIPNames( store )
	if err != nil { return }
	namer = &IPNamer{ exact: map[ string ]string{} , seen: map[ string ]string{} }
	for _ , item := range names {
		if prefix , parse_err := netip.ParsePrefix( item.Target ); parse_err == nil {
			namer.ranges = append( namer.ranges , namedRange{ prefix: prefix , name: item.Name } )
		} else {
			namer.exact[ item.Target ] = item.Name
		}
	}
	sort.SliceStable( namer.ranges , func( i int , j int ) bool {
		return namer.ranges[ i ].prefix.Bits() > namer.ranges[ j ].prefix.Bits()
	} )
	return
}

// Name is what ip is called, or "" if nothing names it.
func ( namer *IPNamer ) Name( ip string ) ( name string ) {
	if ip == "" { return }
	name , known := namer.seen[ ip ]
	if known { return }
	defer func() { namer.seen[ ip ] = name }()
	address , err := netip.ParseAddr( ip )
	if err != nil { return }
	address = address.Unmap().WithZone( "" )
	name , found := namer.exact[ address.String() ]
	if found { return }
	for _ , item := range namer.ranges {
		if item.prefix.Contains( address ) {
			name = item.name
			return
		}
	}
	return
}
