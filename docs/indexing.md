# Kubernetes Proposal - Indexing

## Related Upstream Proposals

| Topic | Link |
| ---- | ---- |
| Etcd Batch GET Support | https://github.com/coreos/etcd/issues/1282 |

## Background

The Kubernetes cluster provides a clear separation between a service (RESTStorage) and its data store (Registry).

The Registry implementation is intended to be pluggable to support alternative data stores, but currently, the
Kubernetes cluster uses etcd as its primary data store.

Etcd provides the ability to look-up resources via the following patterns:

1. Get item by key
2. List descendant items of a resource below a particular key
3. Watch for changes to items at or below a particular key

Unlike many other data stores, Etcd does not currently provide the ability to index items based on a field value for efficient query.

As a result, the Kubernetes architecture has kept the Registry interfaces simple with basic CRUD operations, and when performing List style
operations with a filter, the RESTStorage tier applies the filter over the entire data set on each request by post-filtering each item.
This results in linear filter performance as the number of resources increase.  Alternative Registry implementations that are backed by a 
SQL or No-SQL data store that could support indexing based on field values are unable to leverage that capability to improve filtered queries given 
the current architecture.

This proposal advocates the following:

1. Push filter operations to the Registry tier and out of RESTStorage
2. Define best practice for Etcd-backed registries for indexing data
3. Minimize usage of linear access patterns for common Kubernetes functions when using an Etcd repository

## User stories

1. Ability to build an index over a set of API objects given a projection rule
2. Ability to query index for API object keys that match on a search key
3. Ability to support label selectors and field selectors over Etcd without requring linear access patterns

## Design options

An example scenario to guide discussion,

1. I need to index pods that have a particular label key and value for use in common label selector queries.
2. I need to index pods by an alternate field value (let's say uid), for efficient lookup.

**Option 1: Fan an API object into multiple etcd keys based on indexing criteria on create/update/delete operations**

On create and update, the etcd registry implementation would store a clone of the object in multiple locations.

For example,

| Etcd Indexed Keys |
| ---- |
| /registry/index/pods/label/{labelKey}/{labelValue}/{namespace}/pod1 |
| /registry/index/pods/label/{labelKey}/{labelValue}/{namespace}/pod2 |
| /registry/index/pods/field/{fieldId}/{fieldValue}/{namespace}/pod3 |
| /registry/index/pods/field/{fieldId}/{fieldValue}/{namespace}/pod2 |

In order to support clean-up of the index when an object is updated or deleted, the Etcd implementation would
project the prior version of the object into its set of index keys in order to know what locations to remove and or update.

For registry methods that require a label-selector, or a field-selector, the Etcd backed registry would construct a List
query for each selector and union the results.

Pros:
1. Uses etcd as intended as general purpose key/value store
2. Enables efficient support for = and EXISTS field and label selectors
3. Index is persisted

Cons:
1. Full scan of resources is needed when doing a != field and label selector

**Option 2: Use a WATCH pattern, maintain indexes in memory**

An individual Index manages a set of IndexRecord objects that correlate a particular Value to a Key in a data store.

```
// IndexRecord is the individual entity managed by an Index
type IndexRecord struct {
  // Key is the location in the repository that correlates to this record
  Key string
  // Value is the value that is inserted into the index
  Value string
}
```

An Indexer is responsible for projecting a object into a set of IndexRecord rows.

```
// An indexer is responsible for projecting a node into a set of IndexRecord objects
type Indexer interface {
  // Identifier is the unique label that defines this indexer, used by IndexManager to avoid duplicate indexes being managed
  Identifier() string
  // Reduce projects a node into zero-or-more IndexRecord objects
  Reduce(ob)
  Reduce(object interface{}) ([]IndexRecord, error)
  // TODO need method to get a IndexRecord.Key given an input object, what to do when etcd log resets??
}
```

An Index is used to traverse the set of IndexRecord objects that conform to a particular value.
Index objects are live-updated in response to changes in the repository in the background.

```
type Index interface {
  // Returns true if an IndexRecord exists with the specified key
  Contains(key string) bool
  // Returns true if an IndexRecord exists with the specified value
  Contains(value string) bool
  // ListIndexRecords returns a list of IndexRecord objects that conform to the specified value
  ListIndexRecords(value string) []IndexRecord
  // ListKeys returns a list of keys that match the specified value
  ListKeys(value string) []string
}
```

An IndexManager is responsible for managing Index objects

```
type IndexMananager interface {
  Index(location string, indexer Indexer) (*Index, error)
}
```

The Etcd-backed Registry implementations would implement IndexManager to update and maintain indexes.

As part of setup of the Etcd-backed registry, you could do code like the following:

```
indexManager := ...
pods_uid_index := indexManager.Index("/registry/pods", indexer.NewUidIndexer())
policy_members_index := indexManager.Index("/registry/policy", indexer.NewMembersIndexer())
```

**Option 3: Blend 1 and 2 to persist index in Etcd**

TODO - see if we can blend 1 and 2 so we can persist index
check if WATCH gives old version of resource in all cases