# Kubernetes Prow Job Statistics

This program provides a detailed statistical report on the jobs within the Kubernetes Prow configuration. It offers insights on the total number of jobs, their distribution across clusters, and individual repository statistics.

## Table of Contents

- [Features](#features)
- [Usage](#usage)

## Features

- **Total Job Count**: Quickly get the total count of all jobs present within the configuration.
- **Cluster-wise Statistics**: See the distribution of jobs across different clusters. Understand the spread between Google jobs and community jobs.
- **Repository-wise Statistics**: Get a detailed report of each repository, showing the number of complete, total, remaining jobs, and the completion percentage.

## Usage

1. **Clone the Repository**: Ensure you have the repository containing the program on your local machine.

2. **Navigate to the Directory**: Change your directory to where the program resides.

3. **Compile the Program**:
    ```bash
    go build main.go
    ```

4. **Run the Program**:
    ```bash
    ./main --config=path_to_prow_config --job-config=path_to_job_config --repo-report
    ```

    - `--config`: Path to the prow config. Default: `../../config/prow/config.yaml`
    - `--job-config`: Path to prow job config. Default: `../../config/jobs`
    - `--repo-report`: If set, a detailed report of all repo status will be provided.
