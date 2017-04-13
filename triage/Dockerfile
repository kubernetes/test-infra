FROM jamiehewland/alpine-pypy:2

RUN apk add --no-cache jq
RUN curl -o installer https://sdk.cloud.google.com && bash installer --disable-prompts --install-dir=/ && rm installer && ln -s /google-cloud-sdk/bin/* /bin/

ADD *.py update_summaries.sh /

# Point GOOGLE_APPLICATION_CREDENTIALS at a serviceaccount.json with the necessary permissions.
CMD ["/update_summaries.sh"]
