/*
Copyright 2018 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package ali

import (
	"log"
	"os"
	"reflect"
	"sort"
	"testing"

	"github.com/denverdino/aliyungo/ecs"
	"k8s.io/kops/upup/pkg/fi/cloudup/aliup"
)

var (
	client          *ecs.Client
	TestRegion      = "cn-hongkong"
	TestCluster     = "hello.k8s.local"
	TestExpectedIps = []string{"192.168.168.82", "192.168.168.83", "192.168.168.84"}
)

func init() {
	AccessKeyId := os.Getenv("ALIYUN_ACCESS_KEY_ID")
	AccessKeySecret := os.Getenv("ALIYUN_ACCESS_KEY_SECRET")

	if len(AccessKeyId) != 0 && len(AccessKeySecret) != 0 {
		client = ecs.NewClient(AccessKeyId, AccessKeySecret)
	} else {
		// TODO: error handling
		log.Fatalf("Unable to initialize client")
	}
}

func Test_GetSeeds(t *testing.T) {
	tag := map[string]string{
		aliup.TagClusterName: TestCluster,
	}
	seedProvider, _ := NewSeedProvider(client, TestRegion, tag)
	ips, err := seedProvider.GetSeeds()
	if err != nil {
		t.Fatalf("Unable to get gossip seeds: %v", err)
	}

	sort.Strings(ips)
	sort.Strings(TestExpectedIps)
	if !reflect.DeepEqual(ips, TestExpectedIps) {
		t.Fatalf("Getting seeds conflicts: expect %v, get %v", TestExpectedIps, ips)
	}
}
