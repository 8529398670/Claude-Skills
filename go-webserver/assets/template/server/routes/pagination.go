package routes

import (
	slices "slices"
	sort "sort"
	strconv "strconv"
	strings "strings"
	time "time"

	fiber "github.com/gofiber/fiber/v3"

	mcp "{{MODULE_PATH}}/server/mcp" // feature:mcp
)

// Every list the API hands out is paged the same way: ?limit= and ?offset= in,
// and { "<rows>": [...], "next": <offset of the next page, or null>, "total":
// <rows in all> } out.
//
// An offset rather than a cursor, on purpose. Most of these lists sort on a
// field that is encrypted at rest, so every page has to read the whole list
// anyway and a cursor would save nothing; and the natural cursor for a key or
// a session is its record id, which has no business in a URL. The cost is that
// a row deleted between two pages shifts the next one by a row. The screens
// reload from the first page after any change, so they never see that.
//
// Paging here bounds the response, not the read. See Known limits in
// references/architecture.md.
//
// A list a screen can sort also takes ?sort=<field>&dir=asc|desc, and one it can
// search takes ?q=, both on the whole list before it is cut, since the browser
// only ever holds a page of it. See sendSorted and matchRows.

const (
	pageDefault       = 200
	pageMax           = 1000
	pageLimitMessage  = "limit must be 1-1000"
	pageOffsetMessage = "offset must be 0 or more"
)

// >>> feature:mcp
// pageArgs is the arguments of a paged MCP tool.
type pageArgs struct {
	Limit  *int `json:"limit"`
	Offset *int `json:"offset"`
}
// <<< feature:mcp

// >>> feature:mcp
// pageSchema is the input schema of a paged MCP tool. Both are optional.
var pageSchema = fiber.Map{
	"type": "object",
	"properties": fiber.Map{
		"limit":  fiber.Map{ "type": "integer" , "minimum": 1 , "maximum": pageMax , "description": "Rows to return. Default 200." },
		"offset": fiber.Map{ "type": "integer" , "minimum": 0 , "description": "Rows to skip: the next value of the previous page." },
	},
	"additionalProperties": false,
}
// <<< feature:mcp

// pageBody cuts one page out of rows, which must already be in their final
// order. name is the field the rows go under.
func pageBody[ T any ]( name string , rows []T , limit int , offset int ) ( body fiber.Map ) {
	total := len( rows )
	start := min( offset , total )
	end := min( start + limit , total )
	body = fiber.Map{ name: rows[ start:end ] , "next": nil , "total": total }
	if end < total {
		body[ "next" ] = end
	}
	return
}

// sendPage answers a list route with the page its query asks for.
func sendPage[ T any ]( c fiber.Ctx , name string , rows []T ) ( err error ) {
	limit := pageDefault
	if raw := c.Query( "limit" ); raw != "" {
		parsed , parse_err := strconv.Atoi( raw )
		if parse_err != nil || parsed < 1 || parsed > pageMax {
			err = badRequest( c , pageLimitMessage )
			return
		}
		limit = parsed
	}
	offset := 0
	if raw := c.Query( "offset" ); raw != "" {
		parsed , parse_err := strconv.Atoi( raw )
		if parse_err != nil || parsed < 0 {
			err = badRequest( c , pageOffsetMessage )
			return
		}
		offset = parsed
	}
	err = c.JSON( pageBody( name , rows , limit , offset ) )
	return
}

// matchRows keeps the rows with query in one of fields, in any case. An empty
// query keeps them all. Here rather than in the browser, like sorting.
func matchRows( rows []fiber.Map , query string , fields ...string ) ( matched []fiber.Map ) {
	wanted := strings.ToLower( strings.TrimSpace( query ) )
	if wanted == "" {
		matched = rows
		return
	}
	matched = []fiber.Map{}
	for _ , row := range rows {
		for _ , field := range fields {
			text , is_text := row[ field ].( string )
			if is_text && strings.Contains( strings.ToLower( text ) , wanted ) {
				matched = append( matched , row )
				break
			}
		}
	}
	return
}

// sortKey is one row's value, comparable whatever its type: text in any case,
// dates, numbers and switches. missing is a value the row does not have.
type sortKey struct {
	text    string
	number  float64
	missing bool
}

func sortKeyOf( value any ) ( key sortKey ) {
	switch typed := value.( type ) {
	case string:
		key = sortKey{ text: strings.ToLower( typed ) , missing: typed == "" }
	case time.Time:
		key.number = float64( typed.UnixMicro() )
	case *time.Time:
		if typed == nil {
			key.missing = true
			return
		}
		key.number = float64( typed.UnixMicro() )
	case bool:
		if typed { key.number = 1 }
	case int:
		key.number = float64( typed )
	case int64:
		key.number = float64( typed )
	case uint64:
		key.number = float64( typed )
	default:
		key.missing = true
	}
	return
}

// sortRows orders rows by one field. Rows without it go last whichever way the
// list runs, because "never used" is not the oldest date. Rows that tie stay in
// the list's own order, so a page holds still.
func sortRows( rows []fiber.Map , field string , descending bool ) ( sorted []fiber.Map ) {
	type keyed struct {
		key sortKey
		row fiber.Map
	}
	present := []keyed{}
	absent := []fiber.Map{}
	for _ , row := range rows {
		key := sortKeyOf( row[ field ] )
		if key.missing {
			absent = append( absent , row )
		} else {
			present = append( present , keyed{ key , row } )
		}
	}
	before := func( a sortKey , b sortKey ) ( result bool ) {
		result = a.text < b.text || ( a.text == b.text && a.number < b.number )
		return
	}
	sort.SliceStable( present , func( i int , j int ) bool {
		if descending { return before( present[ j ].key , present[ i ].key ) }
		return before( present[ i ].key , present[ j ].key )
	} )
	sorted = make( []fiber.Map , 0 , len( rows ) )
	for _ , item := range present {
		sorted = append( sorted , item.row )
	}
	sorted = append( sorted , absent... )
	return
}

// sendSorted is sendPage for a list a screen can sort. fields are the row fields
// it may be sorted by, and anything else is refused rather than ignored, so a
// typo does not look like a list in its own order.
func sendSorted( c fiber.Ctx , name string , rows []fiber.Map , fields ...string ) ( err error ) {
	direction := c.Query( "dir" )
	if direction != "" && direction != "asc" && direction != "desc" {
		err = badRequest( c , "dir must be asc or desc" )
		return
	}
	if field := c.Query( "sort" ); field != "" {
		if slices.Contains( fields , field ) == false {
			err = badRequest( c , "sort must be one of: " + strings.Join( fields , ", " ) )
			return
		}
		rows = sortRows( rows , field , direction == "desc" )
	}
	err = sendPage( c , name , rows )
	return
}

// >>> feature:mcp
// toolPage is sendPage for an MCP tool, so the two doors page the same way.
func toolPage[ T any ]( call *mcp.Call , name string , load func() ( []T , error ) ) ( result any , err error ) {
	var args pageArgs
	if err = call.Bind( &args ); err != nil { return }
	limit , offset := pageDefault , 0
	if args.Limit != nil {
		limit = *args.Limit
	}
	if args.Offset != nil {
		offset = *args.Offset
	}
	if limit < 1 || limit > pageMax {
		err = mcp.Refuse( pageLimitMessage )
		return
	}
	if offset < 0 {
		err = mcp.Refuse( pageOffsetMessage )
		return
	}
	rows , err := load()
	if err != nil { return }
	result = pageBody( name , rows , limit , offset )
	return
}
// <<< feature:mcp
