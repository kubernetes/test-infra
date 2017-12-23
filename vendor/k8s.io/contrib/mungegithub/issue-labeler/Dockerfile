FROM ubuntu
Maintainer Garrett Rodrigues "grod@google.com"
RUN apt-get update
RUN apt-get install -y tar git curl nano wget dialog net-tools build-essential vim
RUN apt-get install -y python python-dev python-distribute python-pip
RUN apt-get install -y gfortran libopenblas-dev liblapack-dev
ADD simple_app.py /
ADD stopwords.txt /
ADD requirements.txt /
RUN mkdir models
RUN pip install -r /requirements.txt
EXPOSE 5000
CMD python simple_app.py
