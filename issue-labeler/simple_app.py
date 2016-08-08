#!/usr/bin/env python

# Copyright 2016 The Kubernetes Authors All rights reserved.
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

import os
import simplejson
import logging
from logging.handlers import RotatingFileHandler

import numpy as np
from flask import Flask, request
from sklearn.feature_extraction import FeatureHasher
from sklearn.externals import joblib
from sklearn.linear_model import SGDClassifier
from nltk.tokenize import RegexpTokenizer
from nltk.stem.porter import PorterStemmer
app = Flask(__name__)
#Parameters
team_fn= "./models/trained_teams_model.pkl"
component_fn= "./models/trained_components_model.pkl"
logFile = "/tmp/issue-labeler.log"
logSize = 1024*1024*100
numFeatures = 262144
myLoss = 'hinge'
myAlpha = .1
myPenalty = 'l2'
myHasher = FeatureHasher(input_type="string", n_features= numFeatures, non_negative=True)
myStemmer = PorterStemmer()
tokenizer = RegexpTokenizer(r'\w+')


try:
  if not stopwords:
    stop_fn = "./stopwords.txt"
  with open(stop_fn, 'r') as f:
    stopwords = set([word.strip() for word in f])
except:
  #don't remove any stopwords
  stopwords = []

@app.errorhandler(500)
def internal_error(exception):
  return str(exception), 500

@app.route("/", methods = ["POST"])
def get_labels():
    """
    The request should contain 2 form-urlencoded parameters
      1) title : title of the issue
      2) body: body of the issue
    It returns a team/<label> and a component/<label>
    """
    title = request.form.get('title', "")
    body = request.form.get('body', "")
    tokens = tokenize_stem_stop(" ".join([title, body]))
    team_mod = joblib.load(team_fn)
    comp_mod = joblib.load(component_fn)
    vec = myHasher.transform([tokens])
    tlabel = team_mod.predict(vec)[0]
    clabel = comp_mod.predict(vec)[0]
    return ",".join([tlabel, clabel])


def tokenize_stem_stop(inputString):
    inputString = inputString.encode('utf-8')
    curTitleBody = tokenizer.tokenize(inputString.decode('utf-8').lower())
    return map(myStemmer.stem, filter(lambda x: x not in stopwords, curTitleBody))


@app.route("/update_models", methods = ["PUT"])
def update_model():
    """
    data should contain three fields
      titles: list of titles
      bodies: list of bodies
      labels: list of list of labels
    """
    data = request.json
    titles = data.get('titles')
    bodies = data.get('bodies')
    labels = data.get('labels')

    tTokens = []
    cTokens = []
    team_labels = []
    component_labels = []
    for (title, body, label_list) in zip(titles, bodies, labels):
      tLabel = filter(lambda x: x.startswith('team'), label_list)
      cLabel = filter(lambda x: x.startswith('component'), label_list)
      tokens = tokenize_stem_stop(" ".join([title, body]))
      if tLabel:
        team_labels += tLabel
        tTokens += [tokens]
      if cLabel:
        component_labels += cLabel
        cTokens += [tokens] 
    tVec = myHasher.transform(tTokens)
    cVec = myHasher.transform(cTokens)

    if team_labels:
      if os.path.isfile(team_fn):
        team_model = joblib.load(team_fn)
        team_model.partial_fit(tVec, np.array(team_labels))
      else: 
        #no team model stored so build a new one
        team_model = SGDClassifier(loss=myLoss, penalty=myPenalty, alpha=myAlpha) 
        team_model.fit(tVec, np.array(team_labels))

    if component_labels:
      if os.path.isfile(component_fn):
        component_model = joblib.load(component_fn)
        component_model.partial_fit(cVec, np.array(component_labels))
      else:
        #no comp model stored so build a new one
        component_model = SGDClassifier(loss=myLoss, penalty=myPenalty, alpha=myAlpha) 
        component_model.fit(cVec, np.array(component_labels))
    
    joblib.dump(team_model, team_fn)
    joblib.dump(component_model, component_fn)
    return "" 

def configure_logger():
  FORMAT = '%(asctime)-20s %(levelname)-10s %(message)s'
  file_handler = RotatingFileHandler(logFile, maxBytes=logSize, backupCount=3)
  formatter = logging.Formatter(FORMAT)
  file_handler.setFormatter(formatter)
  app.logger.addHandler(file_handler)

if __name__ == "__main__":
  configure_logger()
  app.run(host="0.0.0.0")
