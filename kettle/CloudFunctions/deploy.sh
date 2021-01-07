GOOGLE_PROJECT_NAME = kubernetes-jenkins
CLOUD_FUNCTION_NAME = HelloGCS  # matches function name above
GCS_BUCKET_NAME = kubernetes-jenkins  # the chosen GCS bucket to read from, a clouf function will need to be deployed for each bucket

MEMORY=256MB  # This is the default; max of 2048MB
TIMEOUT=60s  # This is the default; Max of 540s

DATASET_ID=build  # manually set env var
TABLE_ID=staging  # manually set env var *Needs to be created still

gcloud functions deploy ${CLOUD_FUNCTION_NAME} \
--memory ${MEMORY} \
--retry \
--runtime go113 \
--timeout ${TIMEOUT} \
--trigger-resource ${GCS_BUCKET_NAME} \
--trigger-event google.storage.object.finalize \
--update-env-vars DATASET_ID=${DATASET_ID},TABLE_ID=${TABLE_ID}  # use update instead of set, because it will add or update