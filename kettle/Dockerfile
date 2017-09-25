FROM ubuntu

RUN apt-get update && apt-get install -y curl pv time sqlite3 python-pip && apt-get clean

RUN curl -fsSL https://bitbucket.org/squeaky/portable-pypy/downloads/pypy-5.8-1-linux_x86_64-portable.tar.bz2 | tar xj -C opt
RUN ln -s /opt/pypy*/bin/pypy /usr/bin

ADD requirements.txt /kettle/
RUN pip install -r /kettle/requirements.txt

RUN curl -o installer https://sdk.cloud.google.com && bash installer --disable-prompts --install-dir=/ && rm installer && ln -s /google-cloud-sdk/bin/* /bin/

RUN ln -s /data/build.db /kettle/

ADD *.py schema.json runner.sh /kettle/
ADD buckets.yaml /

CMD ["/kettle/runner.sh"]
VOLUME ["/data"]
