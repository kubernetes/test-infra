#!/usr/bin/env python3

# Copyright 2025 The Kubernetes Authors.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

# pylint: disable=line-too-long,too-many-branches,too-many-statements,too-many-return-statements,too-many-nested-blocks

"""
Kubernetes Audit Log Parser with Swagger/OpenAPI Integration

This script parses a Kubernetes audit log file and generates a list of
Kubernetes API endpoints using the official Swagger/OpenAPI specification
for accurate endpoint naming.

Usage: python3 audit_log_parser.py --audit-logs <audit_log_file>... [--output <output_file>] [--swagger-url <url>]
"""

import argparse
import json
import re
import sys
import time
import urllib.parse
import urllib.request
from collections import Counter
from pathlib import Path

try:
    import yaml

    HAS_YAML = True
except ImportError:
    HAS_YAML = False


def load_endpoint_list_from_yaml(url, endpoint_type="endpoints"):
    """
    Load endpoint lists from Kubernetes conformance YAML files.

    Args:
        url (str): URL or local path to YAML file
        endpoint_type (str): Type of endpoints being loaded (for logging)

    Returns:
        set: Set of endpoint operation IDs
    """
    try:
        print(f"Loading {endpoint_type} from: {url}")
        with urllib.request.urlopen(url, timeout=30) as response:
            content = response.read().decode()

        # Parse YAML content - use PyYAML if available, otherwise manual parsing
        if HAS_YAML:
            try:
                yaml_data = yaml.safe_load(content)
                return _process_yaml_data(yaml_data, endpoint_type)
            except yaml.YAMLError as e:
                print(f"Error: Failed to parse YAML content from {url}")
                print(f"YAML parsing error: {e}")
                print(f"Content preview (first 500 chars):")
                print(content[:500])
                print(f"Cannot proceed with malformed YAML for {endpoint_type}")
                sys.exit(1)
        else:
            print(f"Warning: PyYAML not available, using manual YAML parsing for {endpoint_type}")
            return _manual_yaml_parse(content, endpoint_type)

    except urllib.error.URLError as e:
        print(f"Error: Failed to download {endpoint_type} from {url}")
        print(f"Network error: {e}")
        print(f"Cannot proceed without {endpoint_type} configuration")
        sys.exit(1)
    except Exception as e:  # pylint: disable=broad-except
        print(f"Error: Unexpected failure loading {endpoint_type} from {url}")
        print(f"Error details: {e}")
        print(f"Cannot proceed without {endpoint_type} configuration")
        sys.exit(1)


def _process_yaml_data(yaml_data, endpoint_type):
    """
    Process parsed YAML data to extract endpoint names.

    Args:
        yaml_data: Parsed YAML data structure
        endpoint_type (str): Type of endpoints being loaded
        url (str): URL for error reporting

    Returns:
        set: Set of endpoint operation IDs
    """
    endpoints = set()

    if isinstance(yaml_data, list):
        for item in yaml_data:
            if isinstance(item, dict):
                # Format: list of objects with 'endpoint' key (ineligible_endpoints.yaml)
                if 'endpoint' in item:
                    endpoints.add(item['endpoint'])
            elif isinstance(item, str):
                # Format: simple list of strings (pending_eligible_endpoints.yaml)
                endpoints.add(item)
    else:
        print(f"Error: Expected YAML list format for {endpoint_type}, got {type(yaml_data)}")
        print(f"YAML data structure: {yaml_data}")
        print(f"Cannot proceed with unexpected YAML format for {endpoint_type}")
        sys.exit(1)

    print(f"Loaded {len(endpoints)} {endpoint_type}")
    return endpoints


def _manual_yaml_parse(content, endpoint_type):
    """
    Manual YAML parsing for simple structures when PyYAML is not available.

    Handles two formats:
    1. List of objects: - endpoint: name
    2. Simple list: - name

    Args:
        content (str): YAML content as string
        endpoint_type (str): Type of endpoints being loaded

    Returns:
        set: Set of endpoint operation IDs
    """
    endpoints = set()

    for line in content.split('\n'):
        line = line.strip()

        # Skip empty lines, comments, and document separators
        if not line or line.startswith('#') or line == '---':
            continue

        if line.startswith('- endpoint:'):
            # Format: - endpoint: endpointName (ineligible_endpoints.yaml)
            endpoint = line.replace('- endpoint:', '').strip()
            if endpoint:
                endpoints.add(endpoint)
        elif line.startswith('- ') and ':' not in line:
            # Format: - endpointName (pending_eligible_endpoints.yaml)
            # Only if no colon (to avoid matching other object keys)
            endpoint = line.replace('- ', '').strip()
            if endpoint:
                endpoints.add(endpoint)

    if not endpoints:
        print(f"Warning: No endpoints found using manual YAML parsing for {endpoint_type}")
        print("This might indicate a parsing issue or unexpected YAML format")

    print(f"Loaded {len(endpoints)} {endpoint_type} (manual parsing)")
    return endpoints


def load_ineligible_endpoints(ineligible_endpoints_url=None):
    """
    Load the list of ineligible endpoints from URL or local file.

    Args:
        ineligible_endpoints_url (str, optional): URL or local path to ineligible endpoints YAML file

    Returns:
        set: Set of ineligible endpoint operation IDs to filter out
    """
    if ineligible_endpoints_url is None:
        ineligible_endpoints_url = ("https://raw.githubusercontent.com/kubernetes/kubernetes/"
                                    "master/test/conformance/testdata/ineligible_endpoints.yaml")

    return load_endpoint_list_from_yaml(ineligible_endpoints_url, "ineligible endpoints")


def load_pending_eligible_endpoints(pending_eligible_endpoints_url=None):
    """
    Load the list of pending eligible endpoints from URL or local file.

    Args:
        pending_eligible_endpoints_url (str, optional): URL or local path to pending eligible endpoints YAML file

    Returns:
        set: Set of pending eligible endpoint operation IDs to filter out
    """
    if pending_eligible_endpoints_url is None:
        pending_eligible_endpoints_url = ("https://raw.githubusercontent.com/kubernetes/kubernetes/"
                                          "refs/heads/master/test/conformance/testdata/pending_eligible_endpoints.yaml")

    return load_endpoint_list_from_yaml(pending_eligible_endpoints_url, "pending eligible endpoints")


class SwaggerEndpointMapper:
    """Maps Kubernetes API paths to proper OpenAPI operation IDs using Swagger spec.

    Requires a valid Swagger specification to operate - fails hard if not available.
    """

    def __init__(self, swagger_url=None):
        self.swagger_url = swagger_url or "https://raw.githubusercontent.com/kubernetes/kubernetes/refs/heads/master/api/openapi-spec/swagger.json"
        self.swagger_spec = None
        self.path_to_operation = {}
        self.deprecated_operations = set()
        self.known_resource_types = set()
        self.load_swagger_spec()

    def load_swagger_spec(self):
        """Load and parse the Kubernetes Swagger/OpenAPI specification."""
        print(f"Loading Swagger specification from: {self.swagger_url}")

        try:
            # Try to load from cache first
            cache_file = Path("kubernetes_swagger_cache.json")
            if cache_file.exists():
                cache_age = time.time() - cache_file.stat().st_mtime
                if cache_age < 3600:  # Cache for 1 hour
                    print("Using cached Swagger specification")
                    with open(cache_file, 'r') as f:
                        self.swagger_spec = json.load(f)
                        self._extract_resource_types()
                        self._build_path_mapping()
                        return

            # Download fresh specification
            with urllib.request.urlopen(self.swagger_url, timeout=30) as response:
                self.swagger_spec = json.load(response)

            # Cache the specification
            with open(cache_file, 'w') as f:
                json.dump(self.swagger_spec, f, indent=2)

            print("Swagger specification loaded successfully")
            self._extract_resource_types()
            self._build_path_mapping()

        except urllib.error.URLError as e:
            print(f"Error downloading Swagger spec: {e}")
            print("Cannot proceed without Swagger specification")
            sys.exit(1)
        except json.JSONDecodeError as e:
            print(f"Error parsing Swagger JSON: {e}")
            print("Cannot proceed without valid Swagger specification")
            sys.exit(1)
        except Exception as e:  # pylint: disable=broad-except
            print(f"Unexpected error loading Swagger spec: {e}")
            print("Cannot proceed without Swagger specification")
            sys.exit(1)

    def _build_path_mapping(self):
        """Build mapping from API paths and HTTP methods to OpenAPI operation IDs."""
        if not self.swagger_spec or 'paths' not in self.swagger_spec:
            return

        print("Building path to operation mapping...")

        for path, _ in self.swagger_spec['paths'].items():
            for method, operation in self.swagger_spec['paths'][path].items():
                if method.lower() in ['get', 'post', 'put', 'patch', 'delete'] and 'operationId' in operation:
                    operation_id = operation['operationId']

                    # Check if operation is deprecated
                    description = operation.get('description', '').lower()
                    if 'deprecated' in description:
                        self.deprecated_operations.add(operation_id)

                    # Normalize the path for matching
                    normalized_path = self._normalize_swagger_path(path)
                    key = f"{method.lower()}:{normalized_path}"
                    self.path_to_operation[key] = operation_id

        print(f"Loaded {len(self.path_to_operation)} API operations from Swagger spec")
        if self.deprecated_operations:
            print(f"Found {len(self.deprecated_operations)} deprecated operations")

    def _extract_resource_types(self):
        """Extract resource types from Swagger paths to avoid hardcoding."""
        if not self.swagger_spec or 'paths' not in self.swagger_spec:
            return

        print("Extracting resource types from Swagger specification...")

        # Extract resource types from paths
        for path, _ in self.swagger_spec['paths'].items():
            # Skip non-resource paths
            if not path.startswith('/api') or '/watch/' in path:
                continue

            # Parse path segments
            segments = [s for s in path.split('/') if s]

            for i, segment in enumerate(segments):
                # Look for resource type patterns in paths
                # Resource types are typically:
                # 1. In paths like /api/v1/{resource} or /apis/group/version/{resource}
                # 2. In paths like /api/v1/namespaces/{namespace}/{resource}
                # 3. Plural nouns that aren't parameters

                if (segment and
                        not segment.startswith('{') and
                        '.' not in segment and
                        not segment.startswith('v') and
                        segment not in ['api', 'apis', 'namespaces', 'status', 'scale', 'binding',
                                        'proxy', 'log', 'exec', 'attach', 'portforward', 'eviction',
                                        'ephemeralcontainers', 'finalize', 'watch']):

                    # Check if this looks like a resource type (plural noun at end of path or before {name})
                    next_segment = segments[i + 1] if i + 1 < len(segments) else None

                    # It's likely a resource type if:
                    # 1. It's the last segment in the path (collection operations)
                    # 2. The next segment is {name} or a parameter
                    # 3. It's followed by a subresource
                    if (not next_segment or
                            next_segment.startswith('{') or
                            next_segment in ['status', 'scale', 'binding', 'proxy', 'log', 'exec',
                                             'attach', 'portforward', 'eviction', 'ephemeralcontainers', 'finalize']):

                        # Additional validation: should be a plural noun (heuristic)
                        if len(segment) > 2 and segment.endswith('s') and segment != 'namespaces':
                            self.known_resource_types.add(segment)

        # Add some critical ones that might be missed by heuristics
        critical_resources = {
            'nodes', 'namespaces', 'componentstatuses'  # These don't always follow plural patterns
        }
        self.known_resource_types.update(critical_resources)

        print(f"Extracted {len(self.known_resource_types)} resource types from Swagger spec")

    def _normalize_swagger_path(self, path):  # pylint: disable=no-self-use
        """Normalize Swagger path template for matching against audit log URIs."""
        # Replace common Swagger path parameters with our placeholders
        normalized = path

        # Replace parameter templates
        normalized = re.sub(r'\{namespace\}', '{namespace}', normalized)
        normalized = re.sub(r'\{name\}', '{name}', normalized)
        normalized = re.sub(r'\{node\}', '{node}', normalized)
        normalized = re.sub(r'\{path\}', '{path}', normalized)

        return normalized

    def _normalize_audit_path(self, uri):
        """Normalize audit log URI for matching against Swagger paths."""
        # Remove query parameters
        uri = uri.split('?')[0]

        # Handle API group discovery paths - these should not be normalized
        # Patterns like /apis/apps/, /apis/networking.k8s.io/, etc.
        if re.match(r'^/apis/[^/]+/?$', uri):
            return uri

        # Handle core API discovery paths
        if uri in ['/api/', '/apis/']:
            return uri

        # Replace actual values with parameter placeholders
        normalized = re.sub(r'/namespaces/[^/]+', '/namespaces/{namespace}', uri)
        normalized = re.sub(r'/nodes/[^/]+(?=/|$)', '/nodes/{node}', normalized)

        # Handle proxy paths with additional path segments
        # Convert /proxy/anything/else to /proxy/{path}
        normalized = re.sub(r'/proxy/.*$', '/proxy/{path}', normalized)

        # Replace resource names with {name} placeholder
        # Split the path and process each segment
        parts = normalized.split('/')
        result_parts = []

        for i, part in enumerate(parts):
            if i == 0 and part == '':
                result_parts.append(part)
                continue

            # Skip known path segments that shouldn't be replaced
            if part in ['api', 'apis', 'namespaces', 'status', 'scale', 'binding', 'proxy',
                        'log', 'exec', 'attach', 'portforward', 'eviction', 'ephemeralcontainers',
                        'watch', 'finalize', '{namespace}', '{node}', '{name}']:
                result_parts.append(part)
                continue

            # Skip API group and version segments (contain dots or start with v)
            if '.' in part or re.match(r'^v\d+', part):
                result_parts.append(part)
                continue

            # Determine if this part should be replaced with {name}
            # It should be replaced if it's after a resource type and looks like an instance name
            is_resource_name = False

            if i > 0:
                prev_part = parts[i - 1]

                # This is a resource name if the previous part is a known resource type
                if prev_part in self.known_resource_types:
                    # And this part looks like a resource instance name (not a subresource)
                    if (part not in ['status', 'scale', 'binding', 'proxy', 'log', 'exec',
                                     'attach', 'portforward', 'eviction', 'ephemeralcontainers', 'finalize'] and
                            not part.startswith('{')):
                        is_resource_name = True

            if is_resource_name:
                result_parts.append('{name}')
            else:
                result_parts.append(part)

        return '/'.join(result_parts)

    def _normalize_watch_path(self, uri):
        """Normalize watch operation URI to match Swagger watch path format.

        NOTE: Modern Kubernetes clients use regular resource endpoints with ?watch=true,
        but the OpenAPI spec still defines deprecated /watch/ paths. We need to convert
        from the actual audit log format to the OpenAPI spec format.
        """
        # Remove query parameters first
        clean_uri = uri.split('?')[0]

        # For watch operations, we try both approaches:
        # 1. First try to match the actual path as a regular resource operation
        # 2. Then try the deprecated /watch/ path format

        # The watch parameter in query indicates this is a watch operation,
        # but the path itself is a regular resource path. Most watch operations
        # should be matched as regular GET operations on collections.
        return self._normalize_audit_path(clean_uri)

    def _k8s_verb_to_http_method(self, k8s_verb, uri):  # pylint: disable=unused-argument,no-self-use
        """Convert Kubernetes audit verb to HTTP method for Swagger lookup."""
        k8s_verb = k8s_verb.lower()

        # Map Kubernetes verbs to HTTP methods
        verb_mapping = {
            'get': 'get',
            'list': 'get',
            'watch': 'get',
            'create': 'post',
            'update': 'put',
            'patch': 'patch',
            'delete': 'delete',
            'deletecollection': 'delete',
            'connect': 'get',  # For exec, attach, proxy, etc.
        }

        return verb_mapping.get(k8s_verb, k8s_verb)

    def get_operation_id(self, method, uri):
        """Get the OpenAPI operation ID for a given HTTP method and URI."""
        if not self.swagger_spec:
            return None

        # Check if this is a watch operation by looking at query parameters
        is_watch_operation = 'watch=true' in uri.lower()

        # Normalize the URI (removes query parameters)
        normalized_uri = self._normalize_audit_path(uri)

        # For watch operations, try multiple approaches
        if method.lower() == 'watch' or is_watch_operation:
            # Approach 1: Try the deprecated /watch/ path format from OpenAPI spec
            watch_uri = SwaggerEndpointMapper._convert_to_deprecated_watch_path(normalized_uri)
            watch_key = f"get:{watch_uri}"
            if watch_key in self.path_to_operation:
                return self.path_to_operation[watch_key]

            # Approach 2: Try as a regular GET operation (most common)
            # Watch operations are typically GET requests on collections
            get_key = f"get:{normalized_uri}"
            if get_key in self.path_to_operation:
                return self.path_to_operation[get_key]

            # Approach 3: Try list operations which are often used for watching
            list_variations = [
                get_key,
                f"get:{normalized_uri.rstrip('/')}",
                f"get:{normalized_uri}/" if not normalized_uri.endswith('/') else get_key,
            ]

            for variation in list_variations:
                if variation in self.path_to_operation:
                    return self.path_to_operation[variation]

        # For non-watch operations or if watch matching failed, use regular approach
        http_method = self._k8s_verb_to_http_method(method, uri).lower()
        key = f"{http_method}:{normalized_uri}"

        # Direct match
        if key in self.path_to_operation:
            return self.path_to_operation[key]

        # Try common variations
        variations = [
            key.rstrip('/'),
            key if key.endswith('/') else key + '/',
        ]

        for variation in variations:
            if variation in self.path_to_operation:
                return self.path_to_operation[variation]

        # For specific resource instance operations, try with {name} placeholder
        if '/{name}' not in normalized_uri and http_method == 'get':
            name_variation = f"{http_method}:{normalized_uri}/{{name}}"
            if name_variation in self.path_to_operation:
                return self.path_to_operation[name_variation]

        # Fuzzy matching for complex cases
        return self._fuzzy_match_operation(http_method, normalized_uri)

    def _fuzzy_match_operation(self, method, uri):
        """Try to find a matching operation using fuzzy matching."""
        method_prefix = f"{method}:"

        # Find all operations for this method
        matching_ops = [key for key in self.path_to_operation if key.startswith(method_prefix)]

        # Score matches based on path similarity
        best_match = None
        best_score = 0

        for op_key in matching_ops:
            op_path = op_key[len(method_prefix):]
            score = self._path_similarity(uri, op_path)
            if score > best_score and score > 0.7:  # Require 70% similarity
                best_score = score
                best_match = op_key

        return self.path_to_operation.get(best_match) if best_match else None

    def _path_similarity(self, path1, path2):  # pylint: disable=no-self-use
        """Calculate similarity between two API paths."""
        parts1 = [p for p in path1.split('/') if p]
        parts2 = [p for p in path2.split('/') if p]

        if len(parts1) != len(parts2):
            return 0

        matches = 0
        for p1, p2 in zip(parts1, parts2):
            if p1 == p2 or p1 == '{name}' or p2 == '{name}' or p1 == '{namespace}' or p2 == '{namespace}':
                matches += 1

        return matches / len(parts1) if parts1 else 0

    @staticmethod
    def _convert_to_deprecated_watch_path(uri):
        """Convert a regular resource path to the deprecated /watch/ path format.

        This converts:
        /api/v1/namespaces/{namespace}/pods -> /api/v1/watch/namespaces/{namespace}/pods
        /apis/apps/v1/namespaces/{namespace}/deployments -> /apis/apps/v1/watch/namespaces/{namespace}/deployments
        """
        if uri.startswith('/apis/'):
            # Pattern: /apis/group/version/...
            parts = uri.split('/')
            if len(parts) >= 4:  # /apis/group/version/...
                # Insert 'watch' after version
                new_parts = parts[:4] + ['watch'] + parts[4:]
                return '/'.join(new_parts)
        elif uri.startswith('/api/v1/'):
            # Pattern: /api/v1/...
            # Insert 'watch' after /api/v1
            return uri.replace('/api/v1/', '/api/v1/watch/')

        return uri


def convert_to_k8s_endpoint_fallback(verb,
                                     uri):  # pylint: disable=too-many-branches,too-many-statements,too-many-return-statements
    """
    Fallback method: Convert HTTP verb and URI to Kubernetes endpoint format.
    Used when Swagger specification is not available.
    """
    # Check if this is a watch operation from query parameters
    is_watch_operation = 'watch=true' in uri.lower()

    # Clean the URI by removing query parameters
    clean_uri = uri.split('?')[0]
    clean_uri = re.sub(r'/namespaces/[^/]+', '/namespaces/{namespace}', clean_uri)
    clean_uri = re.sub(r'/nodes/[^/]+', '/nodes/{node}', clean_uri)

    # For watch operations, we prefix with 'watch'
    verb = verb.lower()
    if is_watch_operation or verb == 'watch':
        verb_prefix = 'watch'
        # Use the clean URI for processing
        uri = clean_uri
    else:
        verb_prefix = verb
        uri = clean_uri

    # Handle core API v1
    if uri.startswith('/api/v1/'):
        resource_part = uri[8:]  # Remove /api/v1/

        if resource_part.startswith('namespaces/{namespace}/'):
            remaining = resource_part[23:]
            resource = remaining.split('/')[0]

            if resource and not re.match(r'.*[.-].*[0-9a-f]{8,}', resource) and '.' not in resource:
                parts = remaining.split('/')
                if len(parts) > 2 and parts[1] and not re.match(r'[0-9a-f-]{20,}', parts[1]):
                    subresource = parts[2]
                    if subresource in ['status', 'scale', 'log', 'exec', 'attach', 'portforward', 'proxy', 'binding',
                                       'eviction', 'ephemeralcontainers']:
                        resource_name = resource[0].upper() + resource[1:] if len(resource) > 1 else resource.upper()
                        subresource_name = subresource[0].upper() + subresource[1:] if len(
                            subresource) > 1 else subresource.upper()
                        return f'{verb_prefix}CoreV1Namespaced{resource_name}{subresource_name}'

                resource_name = resource[0].upper() + resource[1:] if len(resource) > 1 else resource.upper()
                return f'{verb_prefix}CoreV1Namespaced{resource_name}'

        else:
            resource = resource_part.split('/')[0]
            if resource and not re.match(r'.*[.-].*[0-9a-f]{8,}', resource) and '.' not in resource:
                parts = resource_part.split('/')
                if len(parts) > 2 and parts[1] and not re.match(r'[0-9a-f-]{20,}', parts[1]):
                    subresource = parts[2]
                    if subresource in ['status', 'scale']:
                        resource_name = resource[0].upper() + resource[1:] if len(resource) > 1 else resource.upper()
                        subresource_name = subresource[0].upper() + subresource[1:] if len(
                            subresource) > 1 else subresource.upper()
                        return f'{verb_prefix}CoreV1{resource_name}{subresource_name}'

                resource_name = resource[0].upper() + resource[1:] if len(resource) > 1 else resource.upper()
                return f'{verb_prefix}CoreV1{resource_name}'

    # Handle APIs group
    elif uri.startswith('/apis/'):
        match = re.match(r'/apis/([^/]+)/([^/]+)/(.*)', uri)
        if match:
            group, version, rest = match.groups()

            group_clean = group.replace('.k8s.io', '').replace('.', '').replace('-', '')
            group_clean = re.sub(r'[^a-zA-Z0-9]', '', group_clean)
            version_clean = version[0].upper() + version[1:] if len(version) > 1 else version.upper()

            if rest.startswith('namespaces/{namespace}/'):
                remaining = rest[23:]
                resource = remaining.split('/')[0]

                if resource and not re.match(r'.*[.-].*[0-9a-f]{8,}', resource):
                    parts = remaining.split('/')
                    if len(parts) > 2 and parts[1] and not re.match(r'[0-9a-f-]{20,}', parts[1]):
                        subresource = parts[2]
                        if subresource in ['status', 'scale', 'binding']:
                            resource_name = resource[0].upper() + resource[1:] if len(
                                resource) > 1 else resource.upper()
                            subresource_name = subresource[0].upper() + subresource[1:] if len(
                                subresource) > 1 else subresource.upper()
                            return f'{verb_prefix}{group_clean.capitalize()}{version_clean}Namespaced{resource_name}{subresource_name}'

                    resource_name = resource[0].upper() + resource[1:] if len(resource) > 1 else resource.upper()
                    return f'{verb_prefix}{group_clean.capitalize()}{version_clean}Namespaced{resource_name}'

            else:
                resource = rest.split('/')[0]
                if resource and not re.match(r'.*[.-].*[0-9a-f]{8,}', resource):
                    parts = rest.split('/')
                    if len(parts) > 2 and parts[1] and not re.match(r'[0-9a-f-]{20,}', parts[1]):
                        subresource = parts[2]
                        if subresource in ['status', 'scale']:
                            resource_name = resource[0].upper() + resource[1:] if len(
                                resource) > 1 else resource.upper()
                            subresource_name = subresource[0].upper() + subresource[1:] if len(
                                subresource) > 1 else subresource.upper()
                            return f'{verb_prefix}{group_clean.capitalize()}{version_clean}{resource_name}{subresource_name}'

                    resource_name = resource[0].upper() + resource[1:] if len(resource) > 1 else resource.upper()
                    return f'{verb_prefix}{group_clean.capitalize()}{version_clean}{resource_name}'

    return None


def parse_audit_logs(file_paths, swagger_mapper=None):  # pylint: disable=too-many-branches,too-many-statements
    """
    Parse multiple audit log files and extract Kubernetes endpoints with counts.

    Args:
        file_paths (list): List of paths to audit log files
        swagger_mapper (SwaggerEndpointMapper): Mapper for converting to operation IDs

    Returns:
        tuple: (Counter of endpoint counts, dict of operation samples, stats dict)
    """
    endpoint_counts = Counter()
    operation_samples = {}  # Store up to 5 audit entries per operation
    total_entries = 0
    skipped_entries = 0
    swagger_matches = 0
    fallback_matches = 0
    total_files = len(file_paths)

    print(f"Parsing {total_files} audit log file(s):")
    for i, file_path in enumerate(file_paths, 1):
        print(f"  [{i}/{total_files}] {file_path}")
    print()

    for file_index, file_path in enumerate(file_paths, 1):
        print(f"Processing file {file_index}/{total_files}: {file_path}")

        try:
            with open(file_path, 'r') as file:
                file_entries = 0
                for line_num, line in enumerate(file, 1):
                    if line_num % 10000 == 0:
                        print(f"  Processed {line_num} lines from {file_path}...")

                    line = line.strip()
                    if not line:
                        continue

                    try:
                        entry = json.loads(line)
                        total_entries += 1
                        file_entries += 1

                        # Only process RequestReceived stage entries
                        stage = entry.get('stage', '')
                        if stage != 'RequestReceived':
                            skipped_entries += 1
                            continue

                        verb = entry.get('verb', '')
                        request_uri = entry.get('requestURI', '')

                        if verb and request_uri:
                            # Check if this is a watch operation based on query parameters
                            # Modern Kubernetes watch operations use ?watch=true parameter
                            is_watch_via_query = 'watch=true' in request_uri.lower()
                            effective_verb = 'watch' if is_watch_via_query else verb
                            # Use Swagger-based mapping (required)
                            operation_id = swagger_mapper.get_operation_id(effective_verb, request_uri)
                            if operation_id:
                                endpoint_counts[operation_id] += 1
                                swagger_matches += 1

                                # Store up to 5 audit samples for this operation
                                if operation_id not in operation_samples:
                                    operation_samples[operation_id] = []
                                if len(operation_samples[operation_id]) < 5:
                                    operation_samples[operation_id].append(entry)
                            else:
                                # Try fallback parsing for edge cases
                                fallback_endpoint = convert_to_k8s_endpoint_fallback(effective_verb, request_uri)
                                if fallback_endpoint:
                                    endpoint_counts[fallback_endpoint] += 1
                                    fallback_matches += 1

                                    # Store up to 5 audit samples for this fallback operation
                                    if fallback_endpoint not in operation_samples:
                                        operation_samples[fallback_endpoint] = []
                                    if len(operation_samples[fallback_endpoint]) < 5:
                                        operation_samples[fallback_endpoint].append(entry)
                                else:
                                    skipped_entries += 1
                        else:
                            skipped_entries += 1

                    except json.JSONDecodeError:
                        skipped_entries += 1
                        continue
                    except Exception:  # pylint: disable=broad-except
                        skipped_entries += 1
                        continue

                print(f"  Completed {file_path}: {file_entries} entries processed")

        except FileNotFoundError:
            print(f"Error: File {file_path} not found")
            continue
        except IOError as e:
            print(f"Error reading file {file_path}: {e}")
            continue

    stats = {
        'total_entries': total_entries,
        'swagger_matches': swagger_matches,
        'fallback_matches': fallback_matches,
        'skipped_entries': skipped_entries,
        'unique_endpoints': len(endpoint_counts),
        'total_api_calls': sum(endpoint_counts.values())
    }

    print(f"\nParsing complete:")
    print(f"  Total log entries: {total_entries}")
    print(f"  Swagger-based matches: {swagger_matches}")
    print(f"  Fallback matches: {fallback_matches}")
    print(f"  Unique endpoints found: {len(endpoint_counts)}")
    print(f"  Total API calls: {sum(endpoint_counts.values())}")
    print(f"  Skipped entries: {skipped_entries}")

    return endpoint_counts, operation_samples, stats


def write_results(endpoint_counts, operation_samples, stats, swagger_mapper=None, output_file=None, sort_by='count',
                  ineligible_endpoints=None, pending_eligible_endpoints=None,
                  audit_operations_json='audit-operations.json'):  # pylint: disable=too-many-statements
    """
    Write results to file or stdout.

    Args:
        endpoint_counts (Counter): Endpoint counts
        operation_samples (dict): Sample audit entries for each operation
        stats (dict): Parsing statistics
        swagger_mapper (SwaggerEndpointMapper): Mapper for finding missing endpoints
        output_file (str, optional): Output file path
        sort_by (str): Sort method - 'count' (descending) or 'name' (alphabetical)
        ineligible_endpoints (set, optional): Set of ineligible endpoints to filter out
        pending_eligible_endpoints (set, optional): Set of pending eligible endpoints to filter out
        audit_operations_json (str): Output path for audit operations JSON file
    """
    if ineligible_endpoints is None:
        ineligible_endpoints = set()
    if pending_eligible_endpoints is None:
        pending_eligible_endpoints = set()

    # Filter out ineligible endpoints, pending eligible endpoints, and deprecated operations from results
    filtered_endpoint_counts = Counter()
    ineligible_found_count = 0
    pending_eligible_found_count = 0
    deprecated_found_count = 0
    deprecated_operations = swagger_mapper.deprecated_operations if swagger_mapper else set()

    for endpoint, count in endpoint_counts.items():
        if endpoint in ineligible_endpoints:
            ineligible_found_count += count
        elif endpoint in pending_eligible_endpoints:
            pending_eligible_found_count += count
        elif endpoint in deprecated_operations:
            deprecated_found_count += count
        else:
            filtered_endpoint_counts[endpoint] = count

    # Update stats to reflect filtering
    filtered_stats = stats.copy()
    filtered_stats['unique_endpoints'] = len(filtered_endpoint_counts)
    filtered_stats['total_api_calls'] = sum(filtered_endpoint_counts.values())
    filtered_stats['ineligible_endpoints_filtered'] = len([ep for ep in endpoint_counts if ep in ineligible_endpoints])
    filtered_stats['ineligible_api_calls_filtered'] = ineligible_found_count
    filtered_stats['pending_eligible_endpoints_filtered'] = len(
        [ep for ep in endpoint_counts if ep in pending_eligible_endpoints])
    filtered_stats['pending_eligible_api_calls_filtered'] = pending_eligible_found_count
    filtered_stats['deprecated_endpoints_filtered'] = len([ep for ep in endpoint_counts if ep in deprecated_operations])
    filtered_stats['deprecated_api_calls_filtered'] = deprecated_found_count
    if sort_by == 'count':
        sorted_endpoints = filtered_endpoint_counts.most_common()
        sort_desc = "sorted by count (descending)"
    elif sort_by == 'name':
        sorted_endpoints = sorted(filtered_endpoint_counts.items(), key=lambda x: x[0].lower())
        sort_desc = "sorted alphabetically"
    else:
        sorted_endpoints = filtered_endpoint_counts.most_common()
        sort_desc = "sorted by count (descending)"

    output = []
    output.append("Kubernetes API Endpoints Found in Audit Log (Swagger-Enhanced)")
    output.append("=" * 70)
    output.append(f"Total unique endpoints: {filtered_stats['unique_endpoints']}")
    output.append(f"Total API calls: {filtered_stats['total_api_calls']}")
    output.append(f"Swagger-based matches: {filtered_stats['swagger_matches']}")
    output.append(f"Fallback matches: {filtered_stats['fallback_matches']}")
    output.append(f"Skipped entries: {filtered_stats['skipped_entries']}")
    if ineligible_endpoints:
        output.append(f"Ineligible endpoints filtered: {filtered_stats['ineligible_endpoints_filtered']}")
        output.append(f"Ineligible API calls filtered: {filtered_stats['ineligible_api_calls_filtered']}")
    if pending_eligible_endpoints:
        output.append(f"Pending eligible endpoints filtered: {filtered_stats['pending_eligible_endpoints_filtered']}")
        output.append(f"Pending eligible API calls filtered: {filtered_stats['pending_eligible_api_calls_filtered']}")
    if deprecated_operations:
        output.append(f"Deprecated endpoints filtered: {filtered_stats['deprecated_endpoints_filtered']}")
        output.append(f"Deprecated API calls filtered: {filtered_stats['deprecated_api_calls_filtered']}")
    output.append(f"Results {sort_desc}")
    output.append("")
    output.append("Endpoint Name (OpenAPI Operation ID) | Count")
    output.append("-" * 70)

    for endpoint, count in sorted_endpoints:
        output.append(f"{endpoint} | {count}")

    # Find and display missing endpoints from Swagger spec
    if swagger_mapper and swagger_mapper.path_to_operation:
        all_swagger_operations = set(swagger_mapper.path_to_operation.values())
        found_operations = set(endpoint_counts.keys())

        # Only count operations that are actually from Swagger (not fallback)
        swagger_found = found_operations & all_swagger_operations
        missing_operations = all_swagger_operations - swagger_found

        # Filter out alpha and beta versions from missing operations
        stable_missing_operations = {
            op for op in missing_operations
            if not any(version in op for version in
                       ['V1alpha', 'V1beta', 'V2alpha', 'V2beta', 'V3alpha', 'V3beta', 'alpha', 'beta'])
        }

        # Filter out ineligible endpoints from missing operations
        if ineligible_endpoints:
            stable_missing_operations = stable_missing_operations - ineligible_endpoints

        # Filter out pending eligible endpoints from missing operations
        if pending_eligible_endpoints:
            stable_missing_operations = stable_missing_operations - pending_eligible_endpoints

        # Filter out deprecated operations from missing operations
        if deprecated_operations:
            stable_missing_operations = stable_missing_operations - deprecated_operations

        if stable_missing_operations:
            filtered_count = len(missing_operations) - len(stable_missing_operations)

            output.append("")
            output.append("=" * 70)
            output.append("STABLE ENDPOINTS NOT FOUND IN AUDIT LOG")
            output.append("=" * 70)
            output.append(f"Total missing stable endpoints: {len(stable_missing_operations)}")
            if filtered_count > 0:
                output.append(
                    f"(Filtered out {filtered_count} alpha/beta/deprecated/ineligible/pending eligible endpoints)")
            output.append(
                f"These are stable, non-deprecated API endpoints defined in the Swagger spec but not exercised in this audit log:")
            output.append("")

            # Sort missing operations alphabetically
            for operation in sorted(stable_missing_operations):
                output.append(f"{operation} | NOT FOUND")

    result_text = "\n".join(output)

    if output_file:
        try:
            with open(output_file, 'w') as f:
                f.write(result_text)
            print(f"\nResults written to: {output_file}")
        except IOError as e:
            print(f"Error writing to file: {e}")
            print("\nResults:")
            print(result_text)
    else:
        print("\nResults:")
        print(result_text)

    # Generate audit-operations.json JSON file with sample audit entries
    _write_audit_operations_json(filtered_endpoint_counts, operation_samples, ineligible_endpoints,
                                 pending_eligible_endpoints, deprecated_operations, audit_operations_json)


def _write_audit_operations_json(filtered_endpoint_counts, operation_samples, ineligible_endpoints,
                                 pending_eligible_endpoints, deprecated_operations,
                                 json_output_path='audit-operations.json'):
    """
    Write audit-operations.json JSON file with sample audit entries for each operation.

    Args:
        filtered_endpoint_counts (Counter): Filtered endpoint counts
        operation_samples (dict): Sample audit entries for each operation
        ineligible_endpoints (set): Set of ineligible endpoints
        pending_eligible_endpoints (set): Set of pending eligible endpoints
        deprecated_operations (set): Set of deprecated operations
        json_output_path (str): Output path for JSON file
    """
    # Build final JSON with samples for operations that passed filtering
    audit_operations_json = {}

    for operation_id in filtered_endpoint_counts:
        # Skip operations that are ineligible, pending eligible, or deprecated
        if (operation_id in ineligible_endpoints or
                operation_id in pending_eligible_endpoints or
                operation_id in deprecated_operations):
            continue

        # Get sample audit entries for this operation (up to 5)
        samples = operation_samples.get(operation_id, [])
        audit_operations_json[operation_id] = samples

    # Write to JSON file
    try:
        with open(json_output_path, 'w', encoding='utf-8') as f:
            json.dump(audit_operations_json, f, indent=2, ensure_ascii=False)

        total_samples = sum(len(samples) for samples in audit_operations_json.values())
        print(
            f"Generated {json_output_path} with {len(audit_operations_json)} operations and {total_samples} sample audit entries")
    except IOError as e:
        print(f"Error writing {json_output_path}: {e}")


def main():
    """Main function to parse command line arguments and run the parser."""
    parser = argparse.ArgumentParser(
        description='Parse Kubernetes audit log using official Swagger/OpenAPI specification',
        formatter_class=argparse.RawDescriptionHelpFormatter,
        epilog="""
Examples:
  python3 audit_log_parser_swagger.py --audit-logs audit.log
  python3 audit_log_parser_swagger.py --audit-logs audit1.log audit2.log
  python3 audit_log_parser_swagger.py --audit-logs audit.log --output results.txt
  python3 audit_log_parser_swagger.py --audit-logs audit*.log --sort count --output results.txt
  python3 audit_log_parser_swagger.py --audit-logs audit.log --swagger-url https://custom-swagger.json
        """
    )

    parser.add_argument('--audit-logs', nargs='+', required=True, help='Path(s) to Kubernetes audit log file(s)')
    parser.add_argument('-o', '--output', help='Output file (default: print to stdout)')
    parser.add_argument('--swagger-url', help='Custom Swagger/OpenAPI specification URL')
    parser.add_argument('--sort', choices=['count', 'name'], default='name',
                        help='Sort results by count (descending) or name (alphabetical). Default: name')
    parser.add_argument('--ineligible-endpoints-url',
                        help='URL or local path to ineligible endpoints YAML file '
                             '(default: https://raw.githubusercontent.com/kubernetes/kubernetes/master/test/conformance/testdata/ineligible_endpoints.yaml)')
    parser.add_argument('--pending-eligible-endpoints-url',
                        help='URL or local path to pending eligible endpoints YAML file '
                             '(default: https://raw.githubusercontent.com/kubernetes/kubernetes/refs/heads/master/test/conformance/testdata/pending_eligible_endpoints.yaml)')
    parser.add_argument('--audit-operations-json',
                        default='audit-operations.json',
                        help='Output path for audit operations JSON file (default: %(default)s)')

    args = parser.parse_args()

    # Load ineligible endpoints for filtering
    ineligible_endpoints = load_ineligible_endpoints(args.ineligible_endpoints_url)

    # Load pending eligible endpoints for filtering
    pending_eligible_endpoints = load_pending_eligible_endpoints(args.pending_eligible_endpoints_url)

    # Initialize Swagger mapper
    swagger_mapper = SwaggerEndpointMapper(args.swagger_url)

    # Verify Swagger spec is loaded (will have already exited if not)
    if not swagger_mapper.swagger_spec:
        print("Error: Failed to load Swagger specification")
        sys.exit(1)

    # Parse the audit log(s)
    endpoint_counts, operation_samples, stats = parse_audit_logs(args.audit_logs, swagger_mapper)

    if not endpoint_counts:
        print("No endpoints found or error parsing file")
        sys.exit(1)

    # Write results
    write_results(endpoint_counts, operation_samples, stats, swagger_mapper, args.output, args.sort,
                  ineligible_endpoints, pending_eligible_endpoints, args.audit_operations_json)


if __name__ == '__main__':
    main()
