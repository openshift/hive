package session

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/vmware/govmomi/vapi/tags"

	"k8s.io/klog/v2"
)

const (
	defaultCacheTTL = time.Hour * 12

	// special value for determine whenever tag or category was found in vCenter during last lookup
	notFoundValue      = "NOT_FOUND"
	notFoundErrMessage = "404 Not Found"
)

type cachedItem struct {
	value     string
	expiresAt int64
}

func (ci cachedItem) expired() bool {
	return time.Now().UnixNano() > ci.expiresAt
}

type cacheMap struct {
	internalMap sync.Map
}

// Set value by the given key with default 12 hours lifetime
func (c *cacheMap) Set(key string, value string) {
	c.SetWithTTL(key, value, defaultCacheTTL)
}

// SetWithTTL sets value by given key with lifetime specified in ttl parameter
func (c *cacheMap) SetWithTTL(key string, value string, ttl time.Duration) {
	c.internalMap.Store(key, cachedItem{value: value, expiresAt: time.Now().Add(ttl).UnixNano()})
}

// Get value by the given key. Second return parameter indicates whenever value found or not
func (c *cacheMap) Get(key string) (value string, ok bool) {
	item, found := c.internalMap.Load(key)
	if !found {
		return "", false
	}
	if item.(cachedItem).expired() {
		klog.V(4).Infof("cache item with name %s expired, invalidating", key)
		c.Delete(key)
		return "", false
	}
	return item.(cachedItem).value, true
}

// Delete value by the given key
func (c *cacheMap) Delete(key string) {
	c.internalMap.Delete(key)
}

type tagsAndCategoriesCache struct {
	tags       cacheMap
	categories cacheMap
}

var sessionAnnotatedCache = map[string]*tagsAndCategoriesCache{}
var sessionCacheMU sync.Mutex

func getOrCreateSessionCache(sessionKey string) *tagsAndCategoriesCache {
	sessionCacheMU.Lock()
	defer sessionCacheMU.Unlock()

	cache, found := sessionAnnotatedCache[sessionKey]
	if !found {
		cache = &tagsAndCategoriesCache{}
		sessionAnnotatedCache[sessionKey] = cache
		return cache
	}
	return cache
}

// CachingTagsManager wraps tags.Manager from vSphere SDK for
// cache mapping between tags or categories name and their ids.
// Reasoning behind this is the implementation details of tags/categories lookup by name,
// to find a tag/category by name vSphere SDK gets a list of ids and then makes an additional request
// for every object till it will not find matched names. Such peculiarity causes a huge performance degradation
// within environments with a large number of tags/categories, because in the worst case number of rest requests for
// object lookup might be equal to the number of objects.
//
// See tags.Manager methods for more details: https://github.com/vmware/govmomi/blob/a2fb82dc55a8eb00932233aa8028ce97140df784/vapi/tags/tags.go#L172
//
// This structure is intended to be used from a Session instance (Session.WithCachingTagsManager method specifically) presented in this module.
type CachingTagsManager struct {
	*tags.Manager

	sessionKey string // different vCenters might have a different tags in it
}

func newTagsCachingClient(tagsManager *tags.Manager, sessionKey string) *CachingTagsManager {
	return &CachingTagsManager{
		tagsManager, sessionKey,
	}
}

// IsName returns true if the id is not an urn.
// this method came from vSphere sdk,
// see https://github.com/vmware/govmomi/blob/a2fb82dc55a8eb00932233aa8028ce97140df784/vapi/tags/tags.go#L121 for
// more context.
func IsName(id string) bool {
	return !strings.HasPrefix(id, "urn:")
}

// isObjectNotFoundErr checks if error message contains "Not Found" message.
// vSphere api client does not expose error type, so we can rely only on error message
func isObjectNotFoundErr(err error) bool {
	return err != nil && strings.HasSuffix(err.Error(), http.StatusText(http.StatusNotFound))
}

// GetTag fetches the tag information for the given identifier.
// The id parameter can be a Tag ID or Tag Name.
// This method shadows original tags.Manager method and caches mapping between
// tag name and its id. In case ID was passed, the original method from tags.Manager would be used.
//
// In case if a tag was not found in vCenter, this would be cached for 12 hours and lookup won't happen till cache expiration.
func (t *CachingTagsManager) GetTag(ctx context.Context, id string) (*tags.Tag, error) {
	if !IsName(id) { // if id is passed no cache check needed, use original GetTag method instantly
		return t.Manager.GetTag(ctx, id)
	}

	cache := getOrCreateSessionCache(t.sessionKey)
	cachedTagID, found := cache.tags.Get(id)
	if found {
		klog.V(4).Infof("tag %s: found cached tag id value", id)
		if cachedTagID == notFoundValue {
			klog.V(4).Infof("tag %s: cache contains special value indicates that tag was not found when cache was filled, treating as non existed tag", id)
			return nil, fmt.Errorf("%s", notFoundErrMessage)
		}

		tag, err := t.Manager.GetTag(ctx, cachedTagID)
		if err != nil {
			if isObjectNotFoundErr(err) {
				klog.V(3).Infof("tag %s: tag was not found in vCenter by cached id, invalidating cache", id)
				// if tag not found, invalidate the cache and fallback to the default search method
				cache.tags.Delete(id)
				return t.Manager.GetTag(ctx, id)
			}
			return tag, err
		}
		return tag, nil
	}

	klog.V(3).Infof("tag %s: tags cache miss, trying to find tag by name, it might take time", id)
	tag, err := t.Manager.GetTag(ctx, id)
	if err != nil {
		if isObjectNotFoundErr(err) {
			klog.V(3).Infof("tag %s not found in vCenter, caching", id)
			// Caching the fact that tag was not found due to performance issues with lookup tag by name.
			// In case when object does not exist amount of requests equals the amount of categories should happen,
			// because of vCenter rest api design.
			// For more context see vcenter rest api documentation and original method implementation:
			// https://developer.vmware.com/apis/vsphere-automation/v7.0U1/cis/rest/com/vmware/cis/tagging/tag/idtag_id/get/
			// https://developer.vmware.com/apis/vsphere-automation/v7.0U1/cis/rest/com/vmware/cis/tagging/tag/get/
			// https://github.com/vmware/govmomi/blob/a2fb82dc55a8eb00932233aa8028ce97140df784/vapi/tags/tags.go#L121
			cache.tags.Set(id, notFoundValue)
		}
		return tag, err
	}
	cache.tags.Set(tag.Name, tag.ID)
	return tag, err
}

// GetCategory fetches the category information for the given identifier.
// The id parameter can be a Category ID or Category Name.
// This method shadows original tags.Manager method and caches mapping between
// tag name and its id. In case ID was passed, the original method from tags.Manager would be used.
//
// In case if a tag was not found in vCenter, this would be cached for 12 hours and lookup won't happen till cache expiration.
func (t *CachingTagsManager) GetCategory(ctx context.Context, id string) (*tags.Category, error) {
	if !IsName(id) { // if id is passed no cache check needed, use original GetTag method instantly
		return t.Manager.GetCategory(ctx, id)
	}

	cache := getOrCreateSessionCache(t.sessionKey)
	cachedCategoryID, found := cache.categories.Get(id)
	if found {
		klog.V(4).Infof("category %s: found cached category id value", id)
		if cachedCategoryID == notFoundValue {
			klog.V(4).Infof("category %s: cache contains special value indicates that tag was not found when cache was filled, treating as non existing category", id)
			return nil, fmt.Errorf("%s", notFoundErrMessage)
		}

		category, err := t.Manager.GetCategory(ctx, cachedCategoryID)
		if err != nil {
			if isObjectNotFoundErr(err) {
				klog.V(3).Infof("category %s: category was not found in vCenter by cached id, invalidating id cache", id)
				cache.categories.Delete(id) // if category not found, invalidate the cache and fallback to the default search method
				return t.Manager.GetCategory(ctx, id)
			}
			return category, err
		}
		return category, nil
	}

	klog.V(3).Infof("category %s: categories cache miss, trying to find category by name, it might take time", id)
	category, err := t.Manager.GetCategory(ctx, id)
	if err != nil {
		if isObjectNotFoundErr(err) {
			klog.V(3).Infof("category %s not found in vCenter, caching", id)
			// Caching the fact that category was not found due to performance issues with lookup category by name.
			// In case when object does not exist amount of requests equals the amount of categories should happen,
			// because of vCenter rest api design.
			// For more context see vcenter rest api documentation and original method implementation:
			// https://developer.vmware.com/apis/vsphere-automation/v7.0U1/cis/rest/com/vmware/cis/tagging/category/idcategory_id/get/
			// https://developer.vmware.com/apis/vsphere-automation/v7.0U1/cis/rest/com/vmware/cis/tagging/category/get/
			// https://github.com/vmware/govmomi/blob/a2fb82dc55a8eb00932233aa8028ce97140df784/vapi/tags/categories.go#L122
			cache.categories.Set(id, notFoundValue)
		}
		return category, err
	}
	cache.categories.Set(category.Name, category.ID)
	return category, err
}

// ListTagsForCategory tag ids for the given category.
// The id parameter can be a Category ID or Category Name.
// Uses caching GetCategory method.
func (t *CachingTagsManager) ListTagsForCategory(ctx context.Context, id string) ([]string, error) {
	if IsName(id) {
		category, err := t.GetCategory(ctx, id)
		if err != nil {
			return nil, err
		}
		id = category.ID
	}
	return t.Manager.ListTagsForCategory(ctx, id)
}
