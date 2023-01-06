/*
This file defines the configuration for the `k8s-prow` cluster:
    - GCP container cluster
    - GCP container node pools
*/

variable "project_name" {
  type    = "string"
  default = "k8s-prow"
}

variable "cluster_name" {
  type    = "string"
  default = "prow"
}

variable "cluster_region" {
  type    = "string"
  default = "us-central1-f"
}

# Configure the Google Cloud provider
provider "google" {
  project = "${var.project_name}"
  region  = "${var.cluster_region}"
}

# Configure the Google Cloud beta provider (required for defining taints)
provider "google-beta" {
  project = "${var.project_name}"
  region  = "${var.cluster_region}"
}

resource "google_container_cluster" "cluster" {
  name     = "${var.cluster_name}"
  location = "${var.cluster_region}"

  # Whether the ABAC authorizer is enabled for this cluster. When enabled, identities
  # in the system, including service accounts, nodes, and controllers, will have statically
  # granted permissions beyond those provided by the RBAC configuration or IAM.
  # Set to `false` to utilize RBAC.
  enable_legacy_abac = true

  # Disable basic and client certificate authorization for the cluster
  master_auth {
    client_certificate_config {
      issue_client_certificate = false
    }
  }
}

# The "ghproxy" pool is for running the GitHub reverse proxy cache (i.e. GHproxy)
resource "google_container_node_pool" "ghproxy_nodes" {
  provider = "google-beta"

  name       = "ghproxy"
  location   = "${google_container_cluster.cluster.location}"
  cluster    = "${google_container_cluster.cluster.name}"
  node_count = 1


  # Auto repair, and auto upgrade nodes to match the master version
  management {
    auto_repair  = true
    auto_upgrade = true
  }

  #  The node configuration of the pool.
  node_config {
    machine_type = "e2-standard-8"
    disk_size_gb = "100"
    labels = {
      dedicated = "ghproxy"
    }
    taint {
      key    = "dedicated"
      value  = "ghproxy"
      effect = "NO_SCHEDULE"
    }
    oauth_scopes = [
      # Compute Engine (rw)
      "https://www.googleapis.com/auth/compute",
      # Storage (ro)
      "https://www.googleapis.com/auth/devstorage.read_only",
      # Service Control (enabled)
      "https://www.googleapis.com/auth/servicecontrol",
      # Service Management (rw)
      "https://www.googleapis.com/auth/service.management",
      # Stackdriver Logging (wo)
      "https://www.googleapis.com/auth/logging.write",
      # Stackdriver Monitoring (full)
      "https://www.googleapis.com/auth/monitoring",
    ]
  }
}

resource "google_container_node_pool" "e2_standard_8_nodes" {
  name       = "e2-standard-8"
  location   = "${google_container_cluster.cluster.location}"
  cluster    = "${google_container_cluster.cluster.name}"
  node_count = 8

  # Auto repair, and auto upgrade nodes to match the master version
  management {
    auto_repair  = true
    auto_upgrade = true
  }

  #  The node configuration of the pool.
  node_config {
    machine_type = "e2-standard-8"
    disk_size_gb = "200"

    oauth_scopes = [
      # Compute Engine (rw)
      "https://www.googleapis.com/auth/compute",
      # Storage (ro)
      "https://www.googleapis.com/auth/devstorage.read_only",
      # Service Control (enabled)
      "https://www.googleapis.com/auth/servicecontrol",
      # Service Management (rw)
      "https://www.googleapis.com/auth/service.management",
      # Stackdriver Logging (wo)
      "https://www.googleapis.com/auth/logging.write",
      # Stackdriver Monitoring (full)
      "https://www.googleapis.com/auth/monitoring",
    ]
  }
}
