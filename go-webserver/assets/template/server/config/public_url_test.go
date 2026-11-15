package config

import (
	testing "testing"
)

// F11: scripts\run.bat and login.bat print server.public_url, and cmd runs
// whatever follows a & | < > ^ % or " in it. Only a plain origin passes.
func TestPublicURLIsAPlainOrigin( t *testing.T ) {
	for _ , value := range []string{
		"https://a&calc&b" , "https://a|b" , `https://a"b` , "https://a%b" , "https://a^b" ,
		"https://a b" , "https://a<b" , "https://a>b" , "https://a!b" , "https://(a)" ,
		"ftp://a.example.com" , "https://a.example.com/path" , "https://a.example.com:port" ,
	} {
		if validPublicURL( value ) { t.Errorf( "%q was accepted" , value ) }
	}
	for _ , value := range []string{
		"" , "https://app.example.com" , "https://app.example.com:8443/" , "http://[::1]:8080" ,
		"http://127.0.0.1:3000" , "http://localhost:8080" ,
	} {
		if validPublicURL( value ) == false { t.Errorf( "%q was refused" , value ) }
	}
}
