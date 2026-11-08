package static

import (
	exec "os/exec"
	testing "testing"
)

// Every theme has to stay readable. The script runs theme.js itself, so this is
// the page's own maths and not a copy of it. Without Node there is nothing to run
// it with.
func TestEveryThemeIsReadable( t *testing.T ) {
	node , err := exec.LookPath( "node" )
	if err != nil { t.Skip( "node is not installed" ) }
	output , err := exec.Command( node , "../../scripts/check_themes.js" ).CombinedOutput()
	if err != nil { t.Fatalf( "%s" , output ) }
}
