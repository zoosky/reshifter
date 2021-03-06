package discovery

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"strings"

	"net/http"
	"net/url"

	"github.com/coreos/etcd/client"
	"github.com/mhausenblas/reshifter/pkg/types"
	"github.com/mhausenblas/reshifter/pkg/util"
)

// ProbeEtcd probes an endpoint at path /version to figure
// which version of etcd it is and in which mode (secure or insecure)
// it is used. Example:
//
//		version, secure, err := ProbeEtcd("http://localhost:2379")
func ProbeEtcd(endpoint string) (string, bool, error) {
	u, err := url.Parse(endpoint + "/version")
	if err != nil {
		return "", false, fmt.Errorf("Can't parse endpoint %s: %s", endpoint, err)
	}
	if u.Scheme == "https" { // secure etcd
		clientcert, clientkey, err := util.ClientCertAndKeyFromEnv()
		if err != nil {
			return "", false, err
		}
		version, verr := getVersionSecure(u.String(), clientcert, clientkey)
		if verr != nil {
			return "", false, verr
		}
		return version, true, nil
	}
	version, verr := getVersion(u.String())
	if verr != nil {
		return "", false, verr
	}
	return version, false, nil
}

// ProbeKubernetesDistro probes an etcd cluster for which Kubernetes
// distribution is present by scanning the available keys.
func ProbeKubernetesDistro(endpoint string) (types.KubernetesDistro, error) {
	version, secure, err := ProbeEtcd(endpoint)
	if err != nil {
		return types.NotADistro, fmt.Errorf("Can't understand endpoint %s: %s", endpoint, err)
	}
	// deal with etcd3 servers:
	if strings.HasPrefix(version, "3") {
		c3, cerr := util.NewClient3(endpoint, secure)
		if cerr != nil {
			return types.NotADistro, fmt.Errorf("Can't connect to etcd: %s", cerr)
		}
		defer func() { _ = c3.Close() }()
		_, err := c3.Get(context.Background(), types.KubernetesPrefix)
		if err != nil {
			return types.NotADistro, nil
		}
		_, err = c3.Get(context.Background(), types.OpenShiftPrefix)
		if err != nil {
			return types.Vanilla, nil
		}
		return types.OpenShift, nil
	}
	// deal with etcd2 servers:
	if strings.HasPrefix(version, "2") {
		c2, cerr := util.NewClient2(endpoint, secure)
		if cerr != nil {
			return types.NotADistro, fmt.Errorf("Can't connect to etcd: %s", cerr)
		}
		kapi := client.NewKeysAPI(c2)
		_, err := kapi.Get(context.Background(), types.KubernetesPrefix, nil)
		if err != nil {
			return types.NotADistro, nil
		}
		_, err = kapi.Get(context.Background(), types.OpenShiftPrefix, nil)
		if err != nil {
			return types.Vanilla, nil
		}
		return types.OpenShift, nil
	}
	return types.NotADistro, fmt.Errorf("Can't determine Kubernetes distro")
}

func getVersion(endpoint string) (string, error) {
	var etcdr types.EtcdResponse
	res, err := http.Get(endpoint)
	if err != nil {
		return "", fmt.Errorf("Can't query %s endpoint: %s", endpoint, err)
	}
	err = json.NewDecoder(res.Body).Decode(&etcdr)
	if err != nil {
		return "", fmt.Errorf("Can't decode response from etcd: %s", err)
	}
	_ = res.Body.Close()
	return etcdr.EtcdServerVersion, nil
}

func getVersionSecure(endpoint, clientcert, clientkey string) (string, error) {
	var etcdr types.EtcdResponse
	cert, err := tls.LoadX509KeyPair(clientcert, clientkey)
	if err != nil {
		return "", err
	}
	tr := &http.Transport{
		TLSClientConfig: &tls.Config{
			Certificates:       []tls.Certificate{cert},
			InsecureSkipVerify: true},
	}
	client := &http.Client{Transport: tr}
	res, err := client.Get(endpoint)
	if err != nil {
		return "", fmt.Errorf("Can't query %s endpoint: %s", endpoint, err)
	}
	err = json.NewDecoder(res.Body).Decode(&etcdr)
	if err != nil {
		return "", fmt.Errorf("Can't decode response from etcd: %s", err)
	}
	_ = res.Body.Close()
	return etcdr.EtcdServerVersion, nil
}
