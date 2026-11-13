package config

import (
	os "os"
	exec "os/exec"
	filepath "path/filepath"
	runtime "runtime"
	testing "testing"
)

// S5: scripts/run.sh keeps .run/ and its log readable by this account alone,
// whatever the umask -- a first run's log holds the owner's login link. A
// .run/ left open by an older run.sh is closed too.
func TestRunDirIsPrivate( t *testing.T ) {
	if runtime.GOOS == "windows" { t.Skip( "run.sh is for unix shells" ) }
	lib , err := filepath.Abs( "../../scripts/lib.sh" )
	if err != nil { t.Fatal( err ) }
	for _ , existing := range []bool{ false , true } {
		dir := t.TempDir()
		run_dir := filepath.Join( dir , ".run" )
		log_file := filepath.Join( run_dir , "server.log" )
		if existing {
			os.MkdirAll( run_dir , 0o755 )
			os.WriteFile( log_file , []byte( "old" ) , 0o644 )
			os.Chmod( run_dir , 0o755 )
			os.Chmod( log_file , 0o644 )
		}
		command := exec.Command( "bash" , "-c" , `umask 022 && source "$1" && private_run_dir` , "bash" , lib )
		command.Dir = dir
		if output , run_err := command.CombinedOutput(); run_err != nil { t.Fatalf( "%v: %s" , run_err , output ) }
		for path , want := range map[string]os.FileMode{ run_dir: 0o700 , log_file: 0o600 } {
			info , stat_err := os.Stat( path )
			if stat_err != nil { t.Fatal( stat_err ) }
			if info.Mode().Perm() != want { t.Errorf( "existing %v: %s is %o, want %o" , existing , path , info.Mode().Perm() , want ) }
		}
	}
}
