// whole-file feature:logs
package middleware

import (
	strings "strings"
	testing "testing"
)

// S6: a Referer naming a login or invite link in any case keeps the
// credential out of the log, as the path itself does.
func TestRefererRedactionIgnoresCase( t *testing.T ) {
	for _ , referer := range []string{ "https://example.com/LOGIN/abc.secret" , "https://example.com/Join/abc.secret" , "https://example.com/login/abc.secret" } {
		if got := redactReferer( referer ); strings.Contains( got , "secret" ) {
			t.Errorf( "%s logged as %s" , referer , got )
		}
	}
}
