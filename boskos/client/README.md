# Boskos Client Library

Boskos client is a go client library interfaces [boskos server](../README.md).

Users of boskos need to use boskos client to communicate with the deployed boskos service.

# Initialize

A boskos client instance is initialized with the URL of target boskos server accompanied with owner of the client.

The client object looks like:

```
type Client struct {
	// RetryCount is the number of times an HTTP request issued by this client
	// is retried when the initial request fails due an inaccessible endpoint.
	RetryCount uint

	// RetryDuration is the interval to wait before retrying an HTTP operation
	// that failed due to an inaccessible endpoint.
	RetryWait time.Duration

	url       string
	resources []string
	owner     string
}
```

To create a boskos client, use `NewClient` and specify the Boskos endpoint URL and resource owner.
The `NewClient` function also sets the client's `RetryCount` to `3` and `RetryWait` interval to `10s`.
```
func NewClient(url string, owner string) *Client
```


# API Reference

```
// Acquire asks boskos for a resource of certain type in certain state, and set the resource to dest state.
func (c *Client) Acquire(rtype string, state string, dest string) (string, error)

// AcquireWait blocks until Acquire returns the specified resource or the
// provided context is cancelled or its deadline exceeded.
func (c *Client) AcquireWait(rtype string, state string, dest string) (string, error)

// AcquireByState asks boskos for a resources of certain type, and set the resource to dest state.
// Returns a list of resources on success.
func (c *Client) AcquireByState(state, dest string, names []string) ([]common.Resource, error)

// AcquireByStateWait blocks until AcquireByState returns the specified
// resource(s) or the provided context is cancelled or its deadline exceeded.
func (c *Client) AcquireByStateWait(ctx context.Context, state, dest string, names []string) ([]common.Resource, error)

// ReleaseAll returns all resources hold by the client back to boskos and set them to dest state.
func (c *Client) ReleaseAll(dest string) error

// ReleaseOne returns one of owned resources back to boskos and set it to dest state.
func (c *Client) ReleaseOne(name string, dest string) error

// UpdateAll signals update for all resources hold by the client.
func (c *Client) UpdateAll(state string) error

// UpdateOne signals update for one of the resources hold by the client.
func (c *Client) UpdateOne(name string, state string) error

// Reset will scan all boskos resources of type, in state, last updated before expire, and set them to dest state.
// Returns a map of {resourceName:owner} for further actions.
func (c *Client) Reset(rtype string, state string, expire time.Duration, dest string) (map[string]string, error)
```
