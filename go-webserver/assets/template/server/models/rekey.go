package models

import (
	json "encoding/json"
	fmt "fmt"

	db "{{MODULE_PATH}}/server/db" // feature:two_factor|webhooks|files
	encryption "{{MODULE_PATH}}/server/encryption"
)

// rekeyFields is every field sealed on its own with the secret key, by bucket.
var rekeyFields = map[string][]string{
	db.BucketUsers:    { "totp_secret" , "totp_pending" }, // feature:two_factor
	db.BucketWebhooks: { "secret" }, // feature:webhooks
	db.BucketFiles:    { "key" }, // feature:files
}

// RekeyRecord is what moving the database to a new key has to do beyond the
// record's own encryption: a TOTP secret is encrypted again on its own (see
// two_factor.go), so it has to be opened with the old key and sealed
// with the new one. Anything else that encrypts a field with the secret key
// belongs here too, or that field is lost at the next rotation.
//
// The record is edited as raw JSON so that fields this code never touches
// come back exactly as they went in.
func RekeyRecord( old_key string , new_key string ) ( rewrite func( bucket string , plain []byte ) ( []byte , error ) ) {
	rewrite = func( bucket string , plain []byte ) ( result []byte , err error ) {
		result = plain
		fields := rekeyFields[ bucket ]
		if len( fields ) == 0 { return }
		record := map[string]json.RawMessage{}
		if err = json.Unmarshal( plain , &record ); err != nil { return }
		changed := false
		for _ , field := range fields {
			raw , present := record[ field ]
			if present == false { continue }
			var sealed string
			if err = json.Unmarshal( raw , &sealed ); err != nil { return }
			if sealed == "" { continue }
			secret , open_err := encryption.ChaChaDecryptBase64String( old_key , sealed )
			if open_err != nil {
				err = fmt.Errorf( "%s: %w" , field , open_err )
				return
			}
			if sealed , err = encryption.ChaChaEncryptString( new_key , secret ); err != nil { return }
			if record[ field ] , err = json.Marshal( sealed ); err != nil { return }
			changed = true
		}
		if changed {
			result , err = json.Marshal( record )
		}
		return
	}
	return
}
