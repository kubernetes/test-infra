# REST API coverage lens

Presents REST endpoints statistics

## Configuration

* `threshold_warning` set threshold for warning highlight
* `threshold_error` set threshold for error highlight

## Expected input

* `uniqueHits` total number of unique params calls (first hit of any leaf should increase this value)
* `expectedUniqueHits` total number of params (leaves)
* `percent` is `uniqueHits` * 100 / `expectedUniqueHits`
* `methodCalled` whether the method was called
* `body` body params
* `query` query params
* `root` root of the tree
* `hits` number of all params hits
* `items` collection of nodes, if not present then the node is a leaf
* `height` height of the tree
* `size` size of the tree

```json
{
    "uniqueHits": 2,
    "expectedUniqueHits": 4,
    "percent": 50.00,
    "endpoints": {
        "/pets": {
            "post": {
                "uniqueHits": 2,
                "expectedUniqueHits": 4,
                "percent": 50.00,
                "methodCalled": true,
                "params": {
                    "body": {
                        "uniqueHits": 2,
                        "expectedUniqueHits": 4,
                        "percent": 50.00,
                        "root": {
                            "hits": 15,
                            "items": {
                                "origin": {
                                   "hits": 8,
                                   "items": {
                                       "country": {
                                           "hits": 8,
                                           "items": {
                                               "name": {
                                                   "hits": 8
                                               },
                                               "region": {
                                                   "hits": 0
                                               }
                                           }
                                       }
                                   }
                                },
                                "color": {
                                    "hits": 0
                                },
                                "type": {
                                    "hits": 7
                                }
                            }
                        },
                        "height": 4,
                        "size": 7
                    }
                }
            }
        }
    }
}
```
