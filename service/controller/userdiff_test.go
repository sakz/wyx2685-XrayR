package controller

import (
	"fmt"
	"github.com/wyx2685/XrayR/api"
	"math/rand"
	"reflect"
	"testing"
)

// Original algorithm, omitting a dead assignment; preserves duplicate-input semantics.
func legacyCompareUserList(old, new *[]api.UserInfo) (deleted, added []api.UserInfo) {
	mSrc := make(map[api.UserInfo]byte) // 按源数组建索引
	mAll := make(map[api.UserInfo]byte) // 源+目所有元素建索引

	var set []api.UserInfo // 交集

	// 1.源数组建立map
	for _, v := range *old {
		mSrc[v] = 0
		mAll[v] = 0
	}
	// 2.目数组中，存不进去，即重复元素，所有存不进去的集合就是并集
	for _, v := range *new {
		l := len(mAll)
		mAll[v] = 1
		if l == len(mAll) { // 存不了，进并集
			set = append(set, v)
		}
	}
	// 3.遍历交集，在并集中找，找到就从并集中删，删完后就是补集（即并-交=所有变化的元素）
	for _, v := range set {
		delete(mAll, v)
	}
	// 4.此时，mall是补集，所有元素去源中找，找到就是删除的，找不到的必定能在目数组中找到，即新加的
	for v := range mAll {
		_, exist := mSrc[v]
		if exist {
			deleted = append(deleted, v)
		} else {
			added = append(added, v)
		}
	}

	return deleted, added
}

func sameUsers(a, b []api.UserInfo) bool {
	if len(a) != len(b) {
		return false
	}
	m := make(map[api.UserInfo]int)
	for _, u := range a {
		m[u]++
	}
	for _, u := range b {
		m[u]--
	}
	for _, n := range m {
		if n != 0 {
			return false
		}
	}
	return true
}

func TestUserDiffCompatibility(t *testing.T) {
	rng := rand.New(rand.NewSource(1))
	pool := []api.UserInfo{{UID: 1}, {UID: 1, Email: "email"}, {UID: 1, UUID: "uuid"}, {UID: 1, Passwd: "password"}, {UID: 1, Port: 123}, {UID: 1, AlterID: 2}, {UID: 1, Method: "method"}, {UID: 1, SpeedLimit: 12}, {UID: 1, DeviceLimit: 2}, {UID: 2}}
	for i := 0; i < 2000; i++ {
		old := make([]api.UserInfo, rng.Intn(30))
		next := make([]api.UserInfo, rng.Intn(30))
		for j := range old {
			old[j] = pool[rng.Intn(len(pool))]
		}
		for j := range next {
			next[j] = pool[rng.Intn(len(pool))]
		}
		copyOld := append([]api.UserInfo{}, old...)
		copyNext := append([]api.UserInfo{}, next...)
		wantD, wantA := legacyCompareUserList(&old, &next)
		gotD, gotA := compareUserList(&old, &next)
		if !sameUsers(wantD, gotD) || !sameUsers(wantA, gotA) {
			t.Fatalf("case %d old=%v new=%v want=%v/%v got=%v/%v", i, old, next, wantD, wantA, gotD, gotA)
		}
		if !reflect.DeepEqual(old, copyOld) || !reflect.DeepEqual(next, copyNext) {
			t.Fatal("input mutated")
		}
	}
}

func TestSplitDeletedUsers(t *testing.T) {
	c := &Controller{Tag: "node_0.0.0.0_443"}
	deleted := []api.UserInfo{
		{UID: 1, Email: "gone@example.test"},
		// Same tag as the re-added entry: only an attribute changed.
		{UID: 2, Email: "respeed@example.test", SpeedLimit: 100},
	}
	added := []api.UserInfo{
		{UID: 2, Email: "respeed@example.test", SpeedLimit: 200},
	}

	gone, emails := c.splitDeletedUsers(deleted, added)
	if len(gone) != 1 || gone[0].UID != 1 {
		t.Fatalf("only the truly removed user must be gone, got %v", gone)
	}
	if len(emails) != 1 || emails[0] != "node_0.0.0.0_443|gone@example.test|1" {
		t.Fatalf("unexpected gone emails: %v", emails)
	}

	gone, emails = c.splitDeletedUsers(deleted, nil)
	if len(gone) != 2 || len(emails) != 2 {
		t.Fatalf("without added users everything stays deleted, got %v", emails)
	}
}

var diffDeleted, diffAdded []api.UserInfo

func BenchmarkCompareUserList(b *testing.B) {
	for _, n := range []int{100, 1000, 10000, 50000} {
		for _, change := range []string{"unchanged", "reordered", "one-percent", "all"} {
			b.Run(fmt.Sprintf("users=%d/%s", n, change), func(b *testing.B) {
				old := make([]api.UserInfo, n)
				next := make([]api.UserInfo, n)
				for i := range old {
					old[i] = api.UserInfo{UID: i, Email: fmt.Sprintf("user%d@example.test", i), UUID: fmt.Sprintf("uuid-%d", i), SpeedLimit: 1000000, DeviceLimit: 2}
					next[i] = old[i]
					if change == "all" || change == "one-percent" && i%100 == 0 {
						next[i].SpeedLimit++
					}
				}
				if change == "reordered" {
					for i, j := 0, n-1; i < j; i, j = i+1, j-1 {
						next[i], next[j] = next[j], next[i]
					}
				}
				b.ReportAllocs()
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					diffDeleted, diffAdded = compareUserList(&old, &next)
				}
			})
		}
	}
}
