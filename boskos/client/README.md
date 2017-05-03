# Boskos Client Library

Boskos client is a go client library interfaces [boskos server](../README.md).

Users of boskos need to use boskos client to communicate with the deployed boskos service.

# Initialize

A boskos client instance is initialized with the URL of target boskos server accompanied with owner of the client.

The client object looks like:

```
type Client struct {
	url       string
	resources []string
	owner     string
}
```

To create a boskos client, use NewClient func, specify boskos server url, and owner of the client:
```
func NewClient(url string, owner string) *Client
```


# API Reference

```
// Acquire asks boskos for a resource of certain type in certain state, and set the resource to dest state.
func (c *Client) Acquire(rtype string, state string, dest string) (string, error)

// ReleaseAll returns all resource hold by the client back to boskos and set them to dest state.
func (c *Client) ReleaseAll(dest string) error

// ReleaseOne returns one of owned resource back to boskos and set it to dest state.
func (c *Client) ReleaseOne(name string, dest string) error

// UpdateAll signals update for all resources hold by the client.
func (c *Client) UpdateAll(state string) error

// UpdateOne signale update for one of the resource hold by the client.
func (c *Client) UpdateOne(name string, state string) error

// Reset will scan all boskos resources of type, in state, last updated before expire, and set them to dest state.
// Returns a map of {resourceName:owner} for further actions.
func (c *Client) Reset(rtype string, state string, expire time.Duration, dest string) (map[string]string, error)
```
