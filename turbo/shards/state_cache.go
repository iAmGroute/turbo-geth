package shards

import (
	"bytes"
	"container/heap"
	"fmt"

	"github.com/google/btree"
	"github.com/ledgerwatch/turbo-geth/common"
	"github.com/ledgerwatch/turbo-geth/core/types/accounts"
)

// LRU state cache consists of two structures - B-Tree and binary heap
// Every element is marked either as Read, Updated, or Deleted via flags

const (
	ModifiedFlag uint16 = 1
	DeletedFlag  uint16 = 2
)

//nolint:maligned
type AccountCacheItem struct {
	address  common.Address
	account  accounts.Account
	sequence int
	queuePos int
	flags    uint16
}

//nolint:maligned
type StorageCacheItem struct {
	address     common.Address
	incarnation uint64
	location    common.Hash
	value       common.Hash
	sequence    int
	queuePos    int
	flags       uint16
}

//nolint:maligned
type CodeCacheItem struct {
	address  common.Address
	code     []byte
	sequence int
	queuePos int
	flags    uint16
}

type CacheItem interface {
	btree.Item
	GetSequence() int
	SetSequence(sequence int)
	GetSize() int
	GetQueuePos() int
	SetQueuePos(pos int)
	HasFlag(flag uint16) bool     // Check if specified flag is set
	SetFlags(flags uint16)        // Set specified flags, but leaves other flags alone
	ClearFlags(flags uint16)      // Clear specified flags, but laves other flags alone
	CopyValueFrom(item CacheItem) // Copy value (not key) from given item
}

func (aci *AccountCacheItem) Less(than btree.Item) bool {
	switch i := than.(type) {
	case *AccountCacheItem:
		return bytes.Compare(aci.address.Bytes(), i.address.Bytes()) < 0
	case *StorageCacheItem:
		c := bytes.Compare(aci.address.Bytes(), i.address.Bytes())
		if c == 0 {
			// Account comes before its storage items
			return true
		}
		return c < 0
	case *CodeCacheItem:
		c := bytes.Compare(aci.address.Bytes(), i.address.Bytes())
		if c == 0 {
			// Account comes before its code
			return true
		}
		return c < 0
	default:
		panic(fmt.Sprintf("unrecognised type of cache item: %T", than))
	}
}

func (aci *AccountCacheItem) GetSequence() int {
	return aci.sequence
}

func (aci *AccountCacheItem) SetSequence(sequence int) {
	aci.sequence = sequence
}

func (aci *AccountCacheItem) GetSize() int {
	return 1
}

func (aci *AccountCacheItem) GetQueuePos() int {
	return aci.queuePos
}

func (aci *AccountCacheItem) SetQueuePos(pos int) {
	aci.queuePos = pos
}

func (aci *AccountCacheItem) HasFlag(flag uint16) bool {
	return aci.flags&flag != 0
}

func (aci *AccountCacheItem) SetFlags(flags uint16) {
	aci.flags |= flags
}

func (aci *AccountCacheItem) ClearFlags(flags uint16) {
	aci.flags &^= flags
}

func (aci *AccountCacheItem) String() string {
	return fmt.Sprintf("AccountItem(address=%x)", aci.address)
}

func (aci *AccountCacheItem) CopyValueFrom(item CacheItem) {
	otherAci, ok := item.(*AccountCacheItem)
	if !ok {
		panic(fmt.Sprintf("expected AccountCacheItem, got %T", item))
	}
	aci.account.Copy(&otherAci.account)
}

func (sci *StorageCacheItem) Less(than btree.Item) bool {
	switch i := than.(type) {
	case *AccountCacheItem:
		c := bytes.Compare(sci.address.Bytes(), i.address.Bytes())
		if c == 0 {
			// Account comes before its storage items
			return false
		}
		return c < 0
	case *StorageCacheItem:
		c := bytes.Compare(sci.address.Bytes(), i.address.Bytes())
		if c == 0 {
			if sci.incarnation == i.incarnation {
				return bytes.Compare(sci.location.Bytes(), i.location.Bytes()) < 0
			}
			return sci.incarnation < i.incarnation
		}
		return c < 0
	case *CodeCacheItem:
		c := bytes.Compare(sci.address.Bytes(), i.address.Bytes())
		if c == 0 {
			// Code comes before storage items
			return false
		}
		return c < 0
	default:
		panic(fmt.Sprintf("unrecognised type of cache item: %T", than))
	}
}

func (sci *StorageCacheItem) GetSequence() int {
	return sci.sequence
}

func (sci *StorageCacheItem) SetSequence(sequence int) {
	sci.sequence = sequence
}

func (sci *StorageCacheItem) GetSize() int {
	return 1
}

func (sci *StorageCacheItem) GetQueuePos() int {
	return sci.queuePos
}

func (sci *StorageCacheItem) SetQueuePos(pos int) {
	sci.queuePos = pos
}

func (sci *StorageCacheItem) HasFlag(flag uint16) bool {
	return sci.flags&flag != 0
}

func (sci *StorageCacheItem) SetFlags(flags uint16) {
	sci.flags |= flags
}

func (sci *StorageCacheItem) ClearFlags(flags uint16) {
	sci.flags &^= flags
}

func (sci *StorageCacheItem) String() string {
	return fmt.Sprintf("StorageItem(address=%x,incarnation=%d,location=%x)", sci.address, sci.incarnation, sci.location)
}

func (cci *CodeCacheItem) Less(than btree.Item) bool {
	switch i := than.(type) {
	case *AccountCacheItem:
		c := bytes.Compare(cci.address.Bytes(), i.address.Bytes())
		if c == 0 {
			// Account before its code
			return false
		}
		return c < 0
	case *StorageCacheItem:
		c := bytes.Compare(cci.address.Bytes(), i.address.Bytes())
		if c == 0 {
			// Code comes before storage items
			return true
		}
		return c < 0
	case *CodeCacheItem:
		return bytes.Compare(cci.address.Bytes(), i.address.Bytes()) < 0
	default:
		panic(fmt.Sprintf("unrecognised type of cache item: %T", than))
	}
}

func (cci *CodeCacheItem) GetSequence() int {
	return cci.sequence
}

func (cci *CodeCacheItem) SetSequence(sequence int) {
	cci.sequence = sequence
}

func (cci *CodeCacheItem) GetSize() int {
	return 1
}

func (cci *CodeCacheItem) GetQueuePos() int {
	return cci.queuePos
}

func (cci *CodeCacheItem) SetQueuePos(pos int) {
	cci.queuePos = pos
}

func (cci *CodeCacheItem) HasFlag(flag uint16) bool {
	return cci.flags&flag != 0
}

func (cci *CodeCacheItem) SetFlags(flags uint16) {
	cci.flags |= flags
}

func (cci *CodeCacheItem) ClearFlags(flags uint16) {
	cci.flags &^= flags
}

func (cci *CodeCacheItem) String() string {
	return fmt.Sprintf("CodeItem(address=%x)", cci.address)
}

// Heaps for reads and writes grow in the opposite direction, while residing in the same space
type ReadHeap struct {
	items []CacheItem
	end   int
}
type WriteHeap struct {
	items []CacheItem
	start int
}

func (rh ReadHeap) Len() int {
	return rh.end
}

func (rh ReadHeap) Less(i, j int) bool {
	return rh.items[i].GetSequence() < rh.items[j].GetSequence()
}

func (rh ReadHeap) Swap(i, j int) {
	// Swap queue positions in the B-tree leaves too
	rh.items[i].SetQueuePos(j)
	rh.items[j].SetQueuePos(i)
	rh.items[i], rh.items[j] = rh.items[j], rh.items[i]
}

func (rh *ReadHeap) Push(x interface{}) {
	// Push and Pop use pointer receivers because they modify the slice's length,
	// not just its contents.
	rh.items[rh.end] = x.(CacheItem)
	rh.end++
}

func (rh *ReadHeap) Pop() interface{} {
	rh.end--
	return rh.items[rh.end]
}

func (wh WriteHeap) Len() int {
	return len(wh.items) - wh.start
}

func (wh WriteHeap) Less(i, j int) bool {
	l := len(wh.items) - 1
	return wh.items[l-i].GetSequence() < wh.items[l-j].GetSequence()
}

func (wh WriteHeap) Swap(i, j int) {
	// Swap queue positions in the B-tree leaves too
	wh.items[i].SetQueuePos(j)
	wh.items[j].SetQueuePos(i)
	l := len(wh.items) - 1
	i = l - i
	j = l - j
	wh.items[i], wh.items[j] = wh.items[j], wh.items[i]
}

func (wh *WriteHeap) Push(x interface{}) {
	// Push and Pop use pointer receivers because they modify the slice's length,
	// not just its contents.
	wh.start--
	wh.items[wh.start] = x.(CacheItem)
}

func (wh *WriteHeap) Pop() interface{} {
	wh.start++
	return wh.items[wh.start-1]
}

type StateCache struct {
	readWrites  *btree.BTree // Mixed reads and writes
	writes      *btree.BTree // Only writes for the effective iteration
	readQueue   ReadHeap
	writeQueue  WriteHeap
	limitReads  int
	limitWrites int
	sequence    int
}

// NewStateCache create a new state cache based on the B-trees of specific degree. The second and the third parameters are the limit on the number of reads and writes to cache, respectively
func NewStateCache(degree int, limitReads, limitWrites int) *StateCache {
	var sc StateCache
	sc.readWrites = btree.New(degree)
	sc.writes = btree.New(degree)
	heapItems := make([]CacheItem, limitReads+limitWrites) // This will be shares between readQueue and writeQueue
	sc.readQueue.items = heapItems
	sc.readQueue.end = 0 // Empty read queue
	sc.writeQueue.items = heapItems
	sc.writeQueue.start = len(heapItems) // Empty write queue
	sc.limitReads = limitReads
	sc.limitWrites = limitWrites
	return &sc
}

func (sc *StateCache) get(key CacheItem) (CacheItem, bool) {
	item := sc.readWrites.Get(key)
	if item == nil {
		return nil, false
	}
	cacheItem := item.(CacheItem)
	if cacheItem.HasFlag(DeletedFlag) {
		return nil, false
	}
	return cacheItem, true
}

// GetAccount searches and account with given address, without modifying any structures
// Second return value is true if such account is found
func (sc *StateCache) GetAccount(address []byte) (*accounts.Account, bool) {
	var key AccountCacheItem
	copy(key.address[:], address)
	if item, ok := sc.get(&key); ok {
		return &item.(*AccountCacheItem).account, true
	}
	return nil, false
}

// GetStorage searches storage item with given address, incarnation, and location, without modifying any structures
// Second return value is true if such item is found
func (sc *StateCache) GetStorage(address []byte, incarnation uint64, location []byte) ([]byte, bool) {
	var key StorageCacheItem
	copy(key.address[:], address)
	key.incarnation = incarnation
	copy(key.location[:], location)
	if item, ok := sc.get(&key); ok {
		return item.(*StorageCacheItem).value.Bytes(), true
	}
	return nil, false
}

// GetCode searches contract code with given address, without modifying any structures
// Second return value is true if such item is found
func (sc *StateCache) GetCode(address []byte) ([]byte, bool) {
	var key CodeCacheItem
	copy(key.address[:], address)
	if item, ok := sc.get(&key); ok {
		return item.(*CodeCacheItem).code, true
	}
	return nil, true
}

func (sc *StateCache) setRead(item CacheItem) {
	if sc.readWrites.Get(item) != nil {
		panic(fmt.Sprintf("item must not be present in the cache before doing setRead: %s", item))
	}
	item.SetQueuePos(0)
	item.SetSequence(sc.sequence)
	sc.readWrites.ReplaceOrInsert(item)
	if sc.readQueue.Len() >= sc.limitReads {
		// Read queue cannot grow anymore, need to evict one element
		sc.readWrites.Delete(sc.readQueue.items[0])
		sc.readQueue.items[0] = item
		heap.Fix(&sc.readQueue, 0)
	} else {
		// Push new element on the read queue
		heap.Push(&sc.readQueue, item)
	}
}

// SetAccountRead adds given account to the cache, marking it as a read (not written)
func (sc *StateCache) SetAccountRead(address []byte, account *accounts.Account) {
	var aci AccountCacheItem
	copy(aci.address[:], address)
	aci.account.Copy(account)
	aci.flags = 0
	sc.setRead(&aci)
}

// SetAccountRead adds given account address to the cache, marking it as a absent
func (sc *StateCache) SetAccountAbsent(address []byte) {
	var aci AccountCacheItem
	copy(aci.address[:], address)
	aci.flags = DeletedFlag
	sc.setRead(&aci)
}

func (sc *StateCache) setWrite(item CacheItem) {
	// Check if this is going to be modification of the existing entry
	if existing := sc.writes.Get(item); existing != nil {

	}
}

// SetAccountWrite adds given account to the cache, marking it as written (cannot be evicted)
func (sc *StateCache) SetAccountWrite(address []byte, account *accounts.Account) {
	// Check if this is going to be modification of the existing entry
	var aci AccountCacheItem
	copy(aci.address[:], address)
	item := sc.writes.Get(&aci)
	if item != nil {
		existing := item.(*AccountCacheItem)
		existing.account.Copy(account)
		existing.sequence = sc.sequence
		sc.sequence++
		existing.flags = ModifiedFlag
		heap.Fix(&sc.writeQueue, existing.queuePos)
		return
	}
	// Now see if there is such item in the readWrite B-tree - then we replace read entry with write entry
	item = sc.readWrites.Get(&aci)
	if item != nil {
		existing := item.(*AccountCacheItem)
		existing.account.Copy(account)
		existing.sequence = sc.sequence
		sc.sequence++
		existing.flags = ModifiedFlag
		// Remove from the reads heap
		heap.Remove(&sc.readQueue, existing.queuePos)
		existing.queuePos = sc.writeQueue.Len()
		heap.Push(&sc.writeQueue, existing)
		return
	}
	if sc.writeQueue.Len() >= sc.limitWrites {
		panic(fmt.Sprintf("writes queue size (%d) must not go over limit (%d). Commit writes before proceeding", sc.writeQueue.Len(), sc.limitWrites))
	}
	if sc.readQueue.Len()+sc.writeQueue.Len() >= sc.limitReads+sc.limitWrites {
		// There is no space available in the queues, need to evict one read element
		sc.readWrites.Delete(heap.Pop(&sc.readQueue).(btree.Item))
	}
	copy(aci.address[:], address)
	aci.account.Copy(account)
	aci.sequence = sc.sequence
	sc.sequence++
	aci.flags = ModifiedFlag
	// Push new element on the write queue
	aci.queuePos = sc.writeQueue.Len()
	heap.Push(&sc.writeQueue, &aci)
	sc.readWrites.ReplaceOrInsert(&aci)
	sc.writes.ReplaceOrInsert(&aci)
}

// SetAccountDelete is very similar to SetAccountWrite with the difference that there no set value
func (sc *StateCache) SetAccountDelete(address []byte) {
	// Check if this is going to be modification of the existing entry
	var aci AccountCacheItem
	copy(aci.address[:], address)
	item := sc.writes.Get(&aci)
	if item != nil {
		existing := item.(*AccountCacheItem)
		existing.account.Reset()
		existing.sequence = sc.sequence
		sc.sequence++
		existing.flags = ModifiedFlag | DeletedFlag
		heap.Fix(&sc.writeQueue, existing.queuePos)
		return
	}
	// Now see if there is such item in the readWrite B-tree - then we replace read entry with write entry
	item = sc.readWrites.Get(&aci)
	if item != nil {
		existing := item.(*AccountCacheItem)
		existing.account.Reset()
		existing.sequence = sc.sequence
		sc.sequence++
		existing.flags = ModifiedFlag | DeletedFlag
		// Remove from the reads heap
		heap.Remove(&sc.readQueue, existing.queuePos)
		existing.queuePos = sc.writeQueue.Len()
		heap.Push(&sc.writeQueue, existing)
		return
	}
	if sc.writeQueue.Len() >= sc.limitWrites {
		panic(fmt.Sprintf("writes queue size (%d) must not go over limit (%d). Commit writes before proceeding", sc.writeQueue.Len(), sc.limitWrites))
	}
	if sc.readQueue.Len()+sc.writeQueue.Len() >= sc.limitReads+sc.limitWrites {
		// There is no space available in the queues, need to evict one read element
		sc.readWrites.Delete(heap.Pop(&sc.readQueue).(btree.Item))
	}
	copy(aci.address[:], address)
	aci.sequence = sc.sequence
	sc.sequence++
	aci.flags = DeletedFlag
	// Push new element on the write queue
	aci.queuePos = sc.writeQueue.Len()
	heap.Push(&sc.writeQueue, &aci)
	sc.readWrites.ReplaceOrInsert(&aci)
	sc.writes.ReplaceOrInsert(&aci)
}

func (sc *StateCache) SetStorageRead(address []byte, incarnation uint64, location []byte, value []byte) {
	var sci StorageCacheItem
	copy(sci.address[:], address)
	sci.incarnation = incarnation
	copy(sci.location[:], location)
	copy(sci.value[:], value)
	sci.flags = 0
	sc.setRead(&sci)
}

func (sc *StateCache) SetStorageAbsent(address []byte, incarnation uint64, location []byte) {
	var sci StorageCacheItem
	copy(sci.address[:], address)
	sci.incarnation = incarnation
	copy(sci.location[:], location)
	sci.flags = DeletedFlag
	sc.setRead(&sci)
}

func (sc *StateCache) SetStorageWrite(address []byte, incarnation uint64, location []byte, value []byte) {
	// Check if this is going to be modification of the existing entry
	var sci StorageCacheItem
	copy(sci.address[:], address)
	sci.incarnation = incarnation
	copy(sci.location[:], location)
	item := sc.writes.Get(&sci)
	if item != nil {
		existing := item.(*StorageCacheItem)
		copy(existing.value[:], value)
		existing.sequence = sc.sequence
		sc.sequence++
		existing.flags = ModifiedFlag
		heap.Fix(&sc.writeQueue, existing.queuePos)
		return
	}
	// Now see if there is such item in the readWrite B-tree - then we replace read entry with write entry
	item = sc.readWrites.Get(&sci)
	if item != nil {
		existing := item.(*StorageCacheItem)
		copy(existing.value[:], value)
		existing.sequence = sc.sequence
		sc.sequence++
		existing.flags = ModifiedFlag
		// Remove from the reads heap
		heap.Remove(&sc.readQueue, existing.queuePos)
		existing.queuePos = sc.writeQueue.Len()
		heap.Push(&sc.writeQueue, existing)
		return
	}
	if sc.writeQueue.Len() >= sc.limitWrites {
		panic(fmt.Sprintf("writes queue size (%d) must not go over limit (%d). Commit writes before proceeding", sc.writeQueue.Len(), sc.limitWrites))
	}
	if sc.readQueue.Len()+sc.writeQueue.Len() >= sc.limitReads+sc.limitWrites {
		// There is no space available in the queues, need to evict one read element
		sc.readWrites.Delete(heap.Pop(&sc.readQueue).(btree.Item))
	}
	copy(sci.address[:], address)
	sci.incarnation = incarnation
	copy(sci.location[:], location)
	copy(sci.value[:], value)
	sci.sequence = sc.sequence
	sc.sequence++
	sci.flags = ModifiedFlag
	// Push new element on the write queue
	sci.queuePos = sc.writeQueue.Len()
	heap.Push(&sc.writeQueue, &sci)
}

func (sc *StateCache) SetStorageDelete(address []byte, incarnation uint64, location []byte) {
	// Check if this is going to be modification of the existing entry
	var sci StorageCacheItem
	copy(sci.address[:], address)
	sci.incarnation = incarnation
	copy(sci.location[:], location)
	item := sc.writes.Get(&sci)
	if item != nil {
		existing := item.(*StorageCacheItem)
		existing.sequence = sc.sequence
		sc.sequence++
		existing.flags = ModifiedFlag | DeletedFlag
		heap.Fix(&sc.writeQueue, existing.queuePos)
		return
	}
	// Now see if there is such item in the readWrite B-tree - then we replace read entry with write entry
	item = sc.readWrites.Get(&sci)
	if item != nil {
		existing := item.(*StorageCacheItem)
		existing.sequence = sc.sequence
		sc.sequence++
		existing.flags = ModifiedFlag | DeletedFlag
		// Remove from the reads heap
		heap.Remove(&sc.readQueue, existing.queuePos)
		existing.queuePos = sc.writeQueue.Len()
		heap.Push(&sc.writeQueue, existing)
		return
	}
	if sc.writeQueue.Len() >= sc.limitWrites {
		panic(fmt.Sprintf("writes queue size (%d) must not go over limit (%d). Commit writes before proceeding", sc.writeQueue.Len(), sc.limitWrites))
	}
	if sc.readQueue.Len()+sc.writeQueue.Len() >= sc.limitReads+sc.limitWrites {
		// There is no space available in the queues, need to evict one read element
		sc.readWrites.Delete(heap.Pop(&sc.readQueue).(btree.Item))
	}
	copy(sci.address[:], address)
	sci.incarnation = incarnation
	copy(sci.location[:], location)
	sci.sequence = sc.sequence
	sc.sequence++
	sci.flags = DeletedFlag
	// Push new element on the write queue
	sci.queuePos = sc.writeQueue.Len()
	heap.Push(&sc.writeQueue, &sci)
}

func (sc *StateCache) SetCodeRead(address []byte, code []byte) {
	var cci CodeCacheItem
	copy(cci.address[:], address)
	cci.code = make([]byte, len(code))
	copy(cci.code, code)
	cci.flags = 0
	sc.setRead(&cci)
}

func (sc *StateCache) SetCodeAbsent(address []byte) {
	var cci CodeCacheItem
	copy(cci.address[:], address)
	cci.flags = DeletedFlag
	sc.setRead(&cci)
}

func (sc *StateCache) SetCodeWrite(address []byte, code []byte) {
	// Check if this is going to be modification of the existing entry
	var cci CodeCacheItem
	copy(cci.address[:], address)
	item := sc.writes.Get(&cci)
	if item != nil {
		existing := item.(*CodeCacheItem)
		existing.code = make([]byte, len(code))
		copy(existing.code, code)
		existing.sequence = sc.sequence
		sc.sequence++
		existing.flags = ModifiedFlag
		heap.Fix(&sc.writeQueue, existing.queuePos)
		return
	}
	// Now see if there is such item in the readWrite B-tree - then we replace read entry with write entry
	item = sc.readWrites.Get(&cci)
	if item != nil {
		existing := item.(*CodeCacheItem)
		existing.code = make([]byte, len(code))
		copy(existing.code, code)
		existing.sequence = sc.sequence
		sc.sequence++
		existing.flags = ModifiedFlag
		// Remove from the reads heap
		heap.Remove(&sc.readQueue, existing.queuePos)
		existing.queuePos = sc.writeQueue.Len()
		heap.Push(&sc.writeQueue, existing)
		return
	}
	if sc.writeQueue.Len() >= sc.limitWrites {
		panic(fmt.Sprintf("writes queue size (%d) must not go over limit (%d). Commit writes before proceeding", sc.writeQueue.Len(), sc.limitWrites))
	}
	if sc.readQueue.Len()+sc.writeQueue.Len() >= sc.limitReads+sc.limitWrites {
		// There is no space available in the queues, need to evict one read element
		sc.readWrites.Delete(heap.Pop(&sc.readQueue).(btree.Item))
	}
	copy(cci.address[:], address)
	cci.code = make([]byte, len(code))
	copy(cci.code, code)
	cci.sequence = sc.sequence
	sc.sequence++
	cci.flags = ModifiedFlag
	// Push new element on the write queue
	cci.queuePos = sc.writeQueue.Len()
	heap.Push(&sc.writeQueue, &cci)
}

func (sc *StateCache) SetCodeDelete(address []byte) {
	// Check if this is going to be modification of the existing entry
	var cci CodeCacheItem
	copy(cci.address[:], address)
	item := sc.writes.Get(&cci)
	if item != nil {
		existing := item.(*CodeCacheItem)
		existing.code = nil
		existing.sequence = sc.sequence
		sc.sequence++
		existing.flags = ModifiedFlag
		heap.Fix(&sc.writeQueue, existing.queuePos)
		return
	}
	// Now see if there is such item in the readWrite B-tree - then we replace read entry with write entry
	item = sc.readWrites.Get(&cci)
	if item != nil {
		existing := item.(*CodeCacheItem)
		existing.code = nil
		existing.sequence = sc.sequence
		sc.sequence++
		existing.flags = ModifiedFlag | DeletedFlag
		// Remove from the reads heap
		heap.Remove(&sc.readQueue, existing.queuePos)
		existing.queuePos = sc.writeQueue.Len()
		heap.Push(&sc.writeQueue, existing)
		return
	}
	if sc.writeQueue.Len() >= sc.limitWrites {
		panic(fmt.Sprintf("writes queue size (%d) must not go over limit (%d). Commit writes before proceeding", sc.writeQueue.Len(), sc.limitWrites))
	}
	if sc.readQueue.Len()+sc.writeQueue.Len() >= sc.limitReads+sc.limitWrites {
		// There is no space available in the queues, need to evict one read element
		sc.readWrites.Delete(heap.Pop(&sc.readQueue).(btree.Item))
	}
	copy(cci.address[:], address)
	cci.code = nil
	cci.sequence = sc.sequence
	sc.sequence++
	cci.flags = ModifiedFlag | DeletedFlag
	// Push new element on the write queue
	cci.queuePos = sc.writeQueue.Len()
	heap.Push(&sc.writeQueue, &cci)
}

func (sc *StateCache) TurnWritesToReads() {
	writeLen := sc.writeQueue.Len() - 1
	sc.writes.Ascend(func(i btree.Item) bool {
		cacheItem := i.(CacheItem)
		cacheItem.SetQueuePos(sc.readQueue.end + writeLen - cacheItem.GetQueuePos())
		i.(CacheItem).ClearFlags(ModifiedFlag)
		return true
	})
	// Merge write queue into the read queue
	copy(sc.readQueue.items[sc.readQueue.end:], sc.writeQueue.items[sc.writeQueue.start:])
	sc.readQueue.end += len(sc.writeQueue.items) - sc.writeQueue.start
	sc.writeQueue.start = len(sc.writeQueue.items)
	heap.Init(&sc.readQueue)
}