package lru

import (
	"github.com/hashicorp/golang-lru/v2/simplelru"
	"sync"
	"time"
)

type layLruItem[V any] struct {
	lock    sync.Mutex
	expires int64
	err     error
	value   V
}

func NewLayLRU[K comparable, V any](size int, successTTL, failedTTL time.Duration, target Target, onEvict EvictCallback[K, V]) *LayLRU[K, V] {
	var cb simplelru.EvictCallback[K, *layLruItem[V]]
	if onEvict != nil {
		cb = func(key K, value *layLruItem[V]) {
			onEvict(key, value.value)
		}
	}
	core, err := simplelru.NewLRU[K, *layLruItem[V]](size, cb)
	if err != nil {
		panic(err)
	}
	return &LayLRU[K, V]{
		core:       core,
		successTTL: successTTL,
		failedTTL:  failedTTL,
		target:     target,
	}
}

type LayLRU[K comparable, V any] struct {
	lock       sync.Mutex
	core       *simplelru.LRU[K, *layLruItem[V]]
	successTTL time.Duration
	failedTTL  time.Duration
	target     Target
}

func (x *LayLRU[K, V]) Get(key K, fetch func() (V, error)) (V, error) {
	x.lock.Lock()
	v, ok := x.core.Get(key)
	if ok {
		x.lock.Unlock()
		v.lock.Lock()
		expires, value, err := v.expires, v.value, v.err
		if expires != 0 && expires > time.Now().UnixMilli() {
			v.lock.Unlock()
			x.target.IncrGetHit()
			return value, err
		}
	} else {
		v = &layLruItem[V]{}
		x.core.Add(key, v)
		v.lock.Lock()
		x.lock.Unlock()
	}
	defer v.lock.Unlock()
	if v.expires > time.Now().UnixMilli() {
		return v.value, v.err
	}
	v.value, v.err = fetch()
	if v.err == nil {
		v.expires = time.Now().Add(x.successTTL).UnixMilli()
		x.target.IncrGetSuccess()
	} else {
		v.expires = time.Now().Add(x.failedTTL).UnixMilli()
		x.target.IncrGetFailed()
	}
	return v.value, v.err
}

func (x *LayLRU[K, V]) Del(key K) bool {
	x.lock.Lock()
	ok := x.core.Remove(key)
	x.lock.Unlock()
	if ok {
		x.target.IncrDelHit()
	} else {
		x.target.IncrDelNotFound()
	}
	return ok
}

func (x *LayLRU[K, V]) Stop() {

}
