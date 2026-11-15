// whole-file feature:mcp
package routes

import (
	json "encoding/json"
	strconv "strconv"
	strings "strings"
	testing "testing"

	models "{{MODULE_PATH}}/server/models"
)

// A bulk route's tool, <tool>_bulk: the same items, shared and results
// through /mcp, and hidden where its route's tool is.

func TestABulkToolRunsEachItem( t *testing.T ) {
	fixture := newMCPFixture( t )
	member := strconv.FormatUint( fixture.member.ID , 10 )
	_ , reply := fixture.post( t , fixture.admin_key , callTool( "admin_set_user_disabled_bulk" , `{"items":[{"user_id":`+member+`},{"user_id":9999}],"shared":{"disabled":true}}` ) )
	failed , text := toolFailed( t , reply )
	if failed { t.Fatalf( "%s" , text ) }
	answer := map[ string ]any{}
	if err := json.Unmarshal( []byte( text ) , &answer ); err != nil { t.Fatalf( "%s: %v" , text , err ) }
	if answer[ "done" ] != float64( 1 ) || answer[ "failed" ] != float64( 1 ) { t.Fatalf( "%v" , answer ) }
	if user , _ := models.GetUser( fixture.store , fixture.member.ID ); user.Disabled() == false { t.Error( "member is still enabled" ) }

	// A shape the tool cannot take is invalid arguments; a count out of
	// bounds is a refusal in the route's words.
	_ , reply = fixture.post( t , fixture.admin_key , callTool( "admin_set_user_disabled_bulk" , `{"items":[{"user_id":1}],"other":1}` ) )
	if rpc_error , _ := reply[ "error" ].( map[ string ]any ); rpc_error == nil || rpc_error[ "code" ] != float64( -32602 ) { t.Errorf( "unknown argument: %v" , reply ) }
	_ , reply = fixture.post( t , fixture.admin_key , callTool( "admin_set_user_disabled_bulk" , `{"items":[]}` ) )
	if failed , text := toolFailed( t , reply ); failed == false || strings.Contains( text , "items must hold 1 to 100 entries" ) == false { t.Errorf( "%v %q" , failed , text ) }

	// A user-scoped key is not told the admin bulk tool exists.
	_ , reply = fixture.post( t , fixture.user_key , rpc( "tools/list" , `{}` ) )
	if names := strings.Join( toolNames( reply ) , " " ); strings.Contains( names , "admin_set_user_disabled_bulk" ) { t.Error( "user key sees an admin bulk tool" ) }
}
