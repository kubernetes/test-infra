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

"""
Kubernetes API Operations Analysis Tool

This script analyzes Kubernetes API coverage by comparing audit logs from CI runs
against Pull Request changes. It automatically discovers the latest CI audit data
from Google Cloud Storage and compares it with local Pull Request audit data to
identify added or removed API operations.

Usage:
    # Auto-discover latest CI data and use default remote swagger
    python3 kubernetes_api_analysis.py

    # Use specific local swagger file
    python3 kubernetes_api_analysis.py --swagger-url /path/to/swagger.json

    # Use specific CI audit file (skip auto-discovery)
    python3 kubernetes_api_analysis.py --ci-file my-ci-audit.txt
"""

import argparse
import json
import os
import subprocess
import sys
import urllib.request


def extract_swagger_operations(swagger_url, output_file):
    """
    Extract all operationIds from Kubernetes swagger/OpenAPI specification.

    Args:
        swagger_url (str): URL or local path to swagger.json file
        output_file (str): Path to write extracted operations list

    Returns:
        list: Sorted list of operation IDs found in the swagger spec
    """
    print("Step 1: Extracting operationIds from swagger.json...")
    print(f"Swagger URL: {swagger_url}")
    print(f"Output file: {output_file}")

    try:
        # Check if it's a URL or local file path
        if swagger_url.startswith(('http://', 'https://')):
            print("Downloading swagger specification...")
            with urllib.request.urlopen(swagger_url) as response:
                swagger_data = json.loads(response.read().decode())
        else:
            # Local file path
            if not os.path.exists(swagger_url):
                print(f"Error: Swagger file not found at {swagger_url}")
                sys.exit(1)
            with open(swagger_url, 'r') as f:
                swagger_data = json.load(f)
    except (json.JSONDecodeError, IOError, urllib.error.URLError) as e:
        print(f"Error reading swagger specification: {e}")
        sys.exit(1)

    operation_ids = set()

    # Extract operationIds from all paths in the OpenAPI specification
    # Each path can have multiple HTTP methods (GET, POST, etc.)
    # Each method should have an operationId that uniquely identifies it
    if 'paths' in swagger_data:
        for methods in swagger_data['paths'].values():
            for method, details in methods.items():
                # Skip 'parameters' as it's not an HTTP method
                if method != 'parameters' and isinstance(details, dict):
                    operation_id = details.get('operationId')
                    if operation_id:
                        operation_ids.add(operation_id)

    # Sort alphabetically for consistent output
    sorted_operations = sorted(operation_ids)

    # Write to file for later use and debugging
    with open(output_file, 'w') as f:
        for op_id in sorted_operations:
            f.write(f"{op_id}\n")

    print(f"Extracted {len(sorted_operations)} operationIds to {output_file}")
    return sorted_operations


def extract_operations_from_audit_file(audit_file, swagger_operations):
    """
    Extract operations from Kubernetes audit log file and filter by valid swagger operations.

    The audit file format is expected to be:
    "OperationId | Count" where the first column contains the Kubernetes API operation ID

    Args:
        audit_file (str): Path to the audit log file
        swagger_operations (set): Set of valid operation IDs from swagger spec

    Returns:
        list: Sorted list of operations found in audit file that exist in swagger
    """
    if not os.path.exists(audit_file):
        print(f"Error: Audit file not found: {audit_file}")
        sys.exit(1)

    operations = set()

    with open(audit_file, 'r') as f:
        for line in f:
            line = line.strip()
            # Parse audit log format: look for lines with " | " delimiter
            # Skip header lines and "NOT FOUND" entries
            if " | " in line and "Endpoint Name" not in line and "NOT FOUND" not in line:
                # Extract operation name (first column before |)
                operation = line.split('|')[0].strip()
                # Only include operations that are defined in the swagger specification
                # This filters out any custom or invalid operation IDs
                if operation and operation in swagger_operations:
                    operations.add(operation)

    return sorted(operations)


def compare_operations(ci_operations, pull_operations):
    """
    Compare API operations between CI baseline and Pull Request changes.

    Args:
        ci_operations (list): Operations found in CI audit log
        pull_operations (list): Operations found in Pull Request audit log

    Returns:
        tuple: (added_operations, removed_operations) - both as sorted lists
    """
    ci_set = set(ci_operations)
    pull_set = set(pull_operations)

    # Operations in Pull but not in CI (newly added API usage)
    added = sorted(pull_set - ci_set)
    # Operations in CI but not in Pull (removed API usage)
    removed = sorted(ci_set - pull_set)

    return added, removed


def find_latest_ci_audit_file():
    """
    Find and download the latest CI audit file from Google Cloud Storage.

    This function replicates the logic from find_last_audit_run.sh:
    1. Lists all CI run directories in GCS bucket
    2. Sorts by timestamp (newest first)
    3. Finds first directory with finished.json (indicating completed run)
    4. Downloads the audit-endpoints.txt file from that run

    Returns:
        str: Local filename of downloaded audit file, or None if not found

    Requires:
        gsutil command-line tool (Google Cloud SDK)
    """
    bucket_path = "gs://kubernetes-ci-logs/logs/ci-audit-kind-conformance"

    print("Searching for latest CI audit run...")
    print(f"Enumerating directories in {bucket_path}...")

    try:
        # Get all directories, sort by timestamp (descending)
        # Directory names are timestamps, so reverse sort gives us newest first
        result = subprocess.run(['gsutil', 'ls', f'{bucket_path}/'],
                                capture_output=True, text=True, check=True)
        directories = sorted(result.stdout.strip().split('\n'), reverse=True)

        # Find the first directory with finished.json (indicates completed CI run)
        for directory in directories:
            directory = directory.strip()
            if not directory:
                continue

            finished_path = f"{directory}finished.json"
            try:
                # Check if finished.json exists in this directory
                subprocess.run(['gsutil', '-q', 'stat', finished_path],
                               capture_output=True, check=True)
                print(f"Found directory with finished.json: {directory}")

                # Check for audit endpoints file in the artifacts
                audit_path = f"{directory}artifacts/audit/audit-endpoints.txt"
                try:
                    subprocess.run(['gsutil', '-q', 'stat', audit_path],
                                   capture_output=True, check=True)
                    print(f"Found audit file at: {audit_path}")

                    # Download the file to local directory with descriptive name
                    local_filename = "ci-audit-kind-conformance-audit-endpoints.txt"
                    subprocess.run(['gsutil', 'cp', audit_path, local_filename],
                                   capture_output=True, check=True)
                    print(f"Downloaded to: {local_filename}")
                    return local_filename

                except subprocess.CalledProcessError:
                    print(f"Audit file not found at: {audit_path}")
                    continue

            except subprocess.CalledProcessError:
                # No finished.json in this directory, continue to next
                continue

        print("No directory with finished.json and audit file found")
        return None

    except subprocess.CalledProcessError as e:
        print(f"Error accessing GCS bucket: {e}")
        return None
    except FileNotFoundError:
        print("Error: gsutil not found. Please install Google Cloud SDK.")
        return None


def create_argument_parser():
    """Create and configure the argument parser."""
    parser = argparse.ArgumentParser(
        description='Kubernetes API Operations Analysis',
        epilog="""
Examples:
  %(prog)s --pull-audit-endpoints my-pull-audit.txt                                      # Use defaults with required pull file
  %(prog)s --swagger-url /path/swagger.json --pull-audit-endpoints my-pr.txt             # Use local swagger file
  %(prog)s --ci-audit-endpoints my-ci-audit.txt --pull-audit-endpoints my-pr.txt         # Skip auto-discovery, specify both files
        """,
        formatter_class=argparse.RawDescriptionHelpFormatter
    )

    default_swagger_url = ("https://raw.githubusercontent.com/kubernetes/kubernetes/"
                           "refs/heads/master/api/openapi-spec/swagger.json")
    parser.add_argument('--swagger-url',
                        default=default_swagger_url,
                        help='Swagger/OpenAPI specification URL or local file path '
                             '(default: %(default)s)')
    parser.add_argument('--ci-audit-endpoints',
                        default=None,
                        help='CI audit endpoints file (default: auto-discover latest from GCS)')
    parser.add_argument('--pull-audit-endpoints',
                        required=True,
                        help='Pull Request audit endpoints file (required)')
    parser.add_argument('--output-file',
                        default="swagger_operations.txt",
                        help='Output file for swagger operations (default: %(default)s)')
    return parser


def display_results(swagger_operations, ci_operations, pull_operations,
                    added_operations, removed_operations, output_file):
    """Display the analysis results."""
    swagger_count = len(swagger_operations)
    ci_count = len(ci_operations)
    pull_count = len(pull_operations)
    added_count = len(added_operations)
    removed_count = len(removed_operations)
    net_change = added_count - removed_count

    print("SUMMARY")
    print("=======")
    print(f"Total Operations in Swagger:  {swagger_count}")
    print(f"Operations in CI:             {ci_count}")
    print(f"Operations in Pull:           {pull_count}")
    print(f"Operations Added:             {added_count}")
    print(f"Operations Removed:           {removed_count}")
    print(f"Net Change:                   {net_change:+d}")
    print()

    print("OPERATIONS ADDED IN PULL (NOT IN CI)")
    print("====================================")
    print(f"Count: {added_count}")
    print()
    if added_operations:
        for i, operation in enumerate(added_operations, 1):
            print(f"{i:3d}. {operation}")
    else:
        print("No operations added.")
    print()

    print("OPERATIONS REMOVED FROM PULL (IN CI BUT NOT PULL)")
    print("=================================================")
    print(f"Count: {removed_count}")
    print()
    if removed_operations:
        for i, operation in enumerate(removed_operations, 1):
            print(f"{i:3d}. {operation}")
    else:
        print("No operations removed.")
    print()

    print("Analysis complete!")
    print("Generated files:")
    print(f"- {output_file} (swagger operations list)")


def main():
    """Main function that orchestrates the API analysis workflow."""
    args = create_argument_parser().parse_args()

    print("Kubernetes API Operations Analysis")
    print("==================================")
    print()

    # Extract operations from swagger specification
    swagger_operations = extract_swagger_operations(args.swagger_url, args.output_file)
    swagger_operations_set = set(swagger_operations)
    print()

    # Determine CI file - auto-discover latest if not specified
    ci_file = args.ci_audit_endpoints
    if ci_file is None:
        print("No CI audit endpoints file specified, auto-discovering latest from GCS...")
        ci_file = find_latest_ci_audit_file()
        if ci_file is None:
            print("Failed to find latest CI audit file. "
                  "Please specify --ci-audit-endpoints manually.")
            sys.exit(1)
        print()

    # Parse and compare audit endpoint files
    print("Step 2: Comparing audit endpoint files...")
    print(f"CI File: {ci_file}")
    print(f"Pull File: {args.pull_audit_endpoints}")
    print()

    print("Extracting operations from audit files (filtering by swagger operations)...")
    ci_operations = extract_operations_from_audit_file(ci_file, swagger_operations_set)
    pull_operations = extract_operations_from_audit_file(args.pull_audit_endpoints,
                                                         swagger_operations_set)

    # Analyze differences and display results
    added_operations, removed_operations = compare_operations(ci_operations, pull_operations)
    display_results(swagger_operations, ci_operations, pull_operations,
                    added_operations, removed_operations, args.output_file)


if __name__ == "__main__":
    main()
