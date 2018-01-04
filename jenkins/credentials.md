In order to configure a service please do the following:

## Acquire the service account credentials

1. Navigate to the [service accounts] for the project that owns the service
   account (kubernetes-jenkins)
  - Go to Google cloud console
  - Open the burger on top left
  - Select the `IAM & Admin` item
  - Select `Service accounts` on the left sidebar
2. Create the service account if it does not already exist.
  - Give it a name: `kubekins`
  - Note the service account id: `kubekins@kubernetes-jenkins.iam.gserviceaccount.com`
  - Check the box to create/furnish a new key
  - Select the `json` type
3. Download the private key for this account
  - Should already have happened if you just created the account
  - Otherwise click the burger on the right
  - Select the `create key` item
  - Select the `json` type
  - Note the location of this file on your computer

## Add the credentials to jenkins

1. Navigate to the global credentials in jenkins at `/credential-store/domain/_/`
  - Go to jenkins
  - `log in`
  - Select `Credentials` on the left sidebar
  - Select `Global credentials (unrestricted)`
2. Upload the credentials
  - Click `Add credentials` on the left sidebar
  - Select `Secret file` in the `Kind` dropdown list.
  - Set the `scope` to include `Global` and/or `all child items`
  - Set the file to the private key you downloaded previously.
  - Set the description to something that will help you find this later.
  - Click the `Advanced` button to show the `ID` field
  - Set the `ID` to something and note this for later.
3. Note the `ID` of these credentials
  - This is the value you selected in the previous step.
  - Alternatively click the secret file with the matching description
  - Click `Update` on the left sidebar.
  - Click `Advanced` to show the `ID`.
  - Remember the `ID` value for these credentials.

## Add the credentials to the job configuration

1. Add the credentials to a wrapper via the `credentials-binding` plugin.
  - Open the [credentials.yaml] file
  - Add/find a wrapper with a credentials-binding wrapper.
  - Add a new file item to this wrapper where:
    - `credential-id` is the `ID` of the credential from the previous step.
    - `variable` is the environment variable containing the path of to this
      file.
    -  Example:
    ```
    wrapper:
      name: foo-wrapper  # Remember this
      wrappers:
      - credentials-binding:
        - file:
            credential-id: 'my-id'  # This is what you selected previously
            value: 'MY_VARIABLE'  # We are using GOOGLE_APPLICATION_CREDENTIALS
    ```
2. Add the wrapper to a job
  - Find the `job`/`project`/`job-template` of interest ([example job])
  - Ensure the item of interest includes the wrapper defined in the previous
    step:
    - Example:
    ```
    job:
      name: 'hello'
      wrappers:
        - foo-wrapper  # this is from above
    ```


[credentials.yaml]: ./job-configs/kubernetes-jenkins/credentials.yaml
[example job]: ./job-configs/kubernetes-jenkins/kubernetes-e2e-gce.yaml#L40
[service accounts]: https://console.developers.google.com/iam-admin/serviceaccounts/project
