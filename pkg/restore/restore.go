package restore

import (
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	log "github.com/Sirupsen/logrus"
	"github.com/coreos/etcd/client"
	"github.com/mhausenblas/reshifter/pkg/discovery"
	"github.com/mhausenblas/reshifter/pkg/types"
	"github.com/mhausenblas/reshifter/pkg/util"
	"github.com/pierrre/archivefile/zip"
)

// Restore takes archive file afile (without file extension) and
// unpacks it into a target directory. It then traverses the target directory
// in the local filesystem and populates an etcd server at endpoint with the
// content of the sub-directories. On success returns the number of keys restored.
// Example:
//
//		krestored, err := Restore("1498055655", "/tmp", "http://localhost:2379")
func Restore(afile, target string, endpoint string) (int, error) {
	numrestored := 0
	err := unarch(filepath.Join(target, afile)+".zip", target)
	if err != nil {
		return numrestored, err
	}
	target, _ = filepath.Abs(filepath.Join(target, afile, "/"))
	version, secure, err := discovery.ProbeEtcd(endpoint)
	if err != nil {
		return 0, fmt.Errorf("Can't understand endpoint %s: %s", endpoint, err)
	}
	if strings.HasPrefix(version, "3") {
		return 0, fmt.Errorf("Endpoint version %s.x not supported", version)
	}
	if strings.HasPrefix(version, "2") {
		c2, err := util.NewClient2(endpoint, secure)
		if err != nil {
			log.WithFields(log.Fields{"func": "Restore"}).Error(fmt.Sprintf("Can't connect to etcd: %s", err))
			return numrestored, fmt.Errorf("Can't connect to etcd: %s", err)
		}
		kapi := client.NewKeysAPI(c2)
		log.WithFields(log.Fields{"func": "Restore"}).Debug(fmt.Sprintf("Operating in target: %s", target))
		err = filepath.Walk(target, func(path string, f os.FileInfo, e error) error {
			log.WithFields(log.Fields{"func": "Restore"}).Debug(fmt.Sprintf("Looking at path: %s, f: %v, err: %v", path, f.Name(), e))
			key, _ := filepath.Rel(target, path)
			key = "/" + strings.Replace(key, types.EscapeColon, ":", -1)
			if f.Name() == types.ContentFile {
				key = filepath.Dir(key)
				c, cerr := ioutil.ReadFile(path)
				if cerr != nil {
					log.WithFields(log.Fields{"func": "Restore"}).Error(fmt.Sprintf("Can't read content file %s: %s", path, cerr))
					return nil
				}
				_, err = kapi.Set(context.Background(), key, string(c), &client.SetOptions{Dir: false, PrevExist: client.PrevNoExist})
				if err != nil {
					log.WithFields(log.Fields{"func": "Restore"}).Error(fmt.Sprintf("Can't restore key %s: %s", key, err))
					return nil
				}
				log.WithFields(log.Fields{"func": "Restore"}).Debug(fmt.Sprintf("Restored key %s from %s", key, path))
				numrestored++
				return nil
			}
			return nil
		})
		if err != nil {
			return 0, fmt.Errorf("Can't traverse directory %s: %s", target, err)
		}
	}
	return numrestored, nil
}

// unarch takes archive file afile and unpacks it into a target directory.
func unarch(afile, target string) error {
	log.WithFields(log.Fields{"func": "unarch"}).Debug(fmt.Sprintf("Unpacking %s into %s", afile, target))
	err := zip.UnarchiveFile(afile, target, func(apath string) {
		log.WithFields(log.Fields{"func": "unarch"}).Debug(fmt.Sprintf("%s", apath))
	})
	if err != nil {
		return fmt.Errorf("Can't unpack archive: %s", err)
	}
	return nil
}
