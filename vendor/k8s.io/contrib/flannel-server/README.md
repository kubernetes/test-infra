# Flannel Server Helper

This pod contains the following components:
* Flannel
* Etcd
* A helper that bridges the 2

## Usage

Mount your network configuration into the pod. Example configuration:
```json
{
    "Network": "192.168.0.0/16",
    "SubnetLen": 24,
    "Backend": {
        "Type": "vxlan",
        "VNI": 1
     }
}
```

Pass the appropriate command line arguments to the flannel-server-helper, for example the rc in this directory has:
```yaml
      - image: gcr.io/google_containers/flannel-server-helper:0.1
        args:
        - --network-config /network.json
        - --etcd-prefix /kubernetes.io/network
        - --etcd-server http://127.0.0.1:4001
```

This will startup flannel in server mode, and point it at etcd running on port 4001. It will also start the flannel-server-helper, which reads network configuration from /network.json and writes it to etcd. The flannel server will poll etcd till this network configuration is available. Note that the provided rc configures flannel to listen for remote connections on 10253, so the flannel daemon needs to run on each node that's a part of the overlay, with `-remote` pointing at the server.

## Wishlist

* Flannel server helper health checks etcd and flannel
