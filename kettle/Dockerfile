FROM pypy:2

ADD requirements.txt /kettle/
RUN pip install -r /kettle/requirements.txt

RUN apt-get update && apt-get install pv time sqlite3 && apt-get clean

RUN curl -o installer https://sdk.cloud.google.com && bash installer --disable-prompts --install-dir=/ && rm installer && ln -s /google-cloud-sdk/bin/* /bin/

RUN ln -s /data/build.db /kettle/

# stupid hack to deal with debian python2 getsitepackages vs pypy's version
RUN bash -c 'touch /usr/local/site-packages/google/{__init__.py,cloud/__init__.py,cloud/pubsub/__init__.py}'

ADD *.py schema.json runner.sh /kettle/
ADD buckets.yaml /

ENV PYTHONPATH /usr/local/site-packages
CMD ["/kettle/runner.sh"]
VOLUME ["/data"]
