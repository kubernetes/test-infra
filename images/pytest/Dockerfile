FROM python:3.8-slim-buster

WORKDIR /app

COPY metrics/get_jq.sh /app/get_jq.sh
COPY requirements3.txt /app/requirements3.txt

RUN /bin/bash -c "apt-get update && apt-get install -y make wget"
RUN /bin/bash -c "pip3 install -r requirements3.txt"
RUN /bin/bash -c "pip3 install certifi chardet google-api-core google-cloud-bigquery google-cloud-pubsub idna python-dateutil pytz requests six urllib3"
RUN /bin/bash -c "chmod a+x get_jq.sh && ./get_jq.sh && mv jq-1.5 .."

CMD ["make", "pytests"]