// whole-file feature:two_factor
package models

import (
	json "encoding/json"
	filepath "path/filepath"
	testing "testing"

	bolt "go.etcd.io/bbolt"

	config "{{MODULE_PATH}}/server/config"
	db "{{MODULE_PATH}}/server/db"
	encryption "{{MODULE_PATH}}/server/encryption"
)

// S12: opening a database sealed the old way also moves the fields a model
// seals on its own (here a two-factor secret, sealed with secret.key itself)
// to their own subkey, so the account can still sign in.
func TestResealingMovesAFieldToItsSubkey( t *testing.T ) {
	cfg := &config.Config{ DatabasePath: filepath.Join( t.TempDir() , "app.db" ) , SecretKey: encryption.GenerateKeyHex() , EncryptAtRest: true }
	secret := encryption.TOTPNewSecret()
	sealed_secret , err := encryption.ChaChaEncryptString( cfg.SecretKey , secret )
	if err != nil { t.Fatal( err ) }
	handle , err := bolt.Open( cfg.DatabasePath , 0o600 , nil )
	if err != nil { t.Fatal( err ) }
	err = handle.Update( func( tx *bolt.Tx ) ( tx_err error ) {
		users , tx_err := tx.CreateBucketIfNotExists( []byte( db.BucketUsers ) )
		if tx_err != nil { return }
		raw , _ := json.Marshal( User{ ID: 1 , DisplayName: "Owner" , Role: RoleAdmin , TOTPSecret: sealed_secret } )
		record , tx_err := encryption.ChaChaEncryptBytes( cfg.SecretKey , raw )
		if tx_err != nil { return }
		tx_err = users.Put( encryption.Uint64ToBytes( 1 ) , record )
		return
	} )
	handle.Close()
	if err != nil { t.Fatal( err ) }

	store , err := db.Open( cfg )
	if err != nil { t.Fatal( err ) }
	defer store.Close()
	user , err := GetUser( store , 1 )
	if err != nil { t.Fatal( err ) }
	opened , err := encryption.ChaChaDecryptBase64String( encryption.SubKey( cfg.SecretKey , encryption.PurposeTOTP ) , user.TOTPSecret )
	if err != nil || opened != secret { t.Fatalf( "two-factor secret after re-sealing: %q %v" , opened , err ) }
}
