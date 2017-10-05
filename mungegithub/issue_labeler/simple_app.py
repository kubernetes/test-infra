#!/usr/bin/env python

# Copyright 2016 The Kubernetes Authors.
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
import logging
from logging.handlers import RotatingFileHandler

# pylint: disable=import-error
import numpy as np
from flask import Flask, request
from sklearn.feature_extraction import FeatureHasher
from sklearn.externals import joblib
from sklearn.linear_model import SGDClassifier
from nltk.tokenize import RegexpTokenizer
from nltk.stem.porter import PorterStemmer
# pylint: enable=import-error

APP = Flask(__name__)
# parameters
TEAM_FN = './models/trained_teams_model.pkl'
COMPONENT_FN = './models/trained_components_model.pkl'
LOG_FILE = '/tmp/issue-labeler.log'
LOG_SIZE = 1024*1024*100
NUM_FEATURES = 262144
MY_LOSS = 'hinge'
MY_ALPHA = .1
MY_PENALTY = 'l2'
MY_HASHER = FeatureHasher(input_type='string', n_features=NUM_FEATURES, non_negative=True)
MY_STEMMER = PorterStemmer()
TOKENIZER = RegexpTokenizer(r'\w+')

STOPWORDS = []
try:
    if not STOPWORDS:
        STOPWORDS_FILENAME = './stopwords.txt'
    with open(STOPWORDS_FILENAME, 'r') as fp:
        STOPWORDS = list([word.strip() for word in fp])
except: # pylint:disable=bare-except
    # don't remove any stopwords
    STOPWORDS = []

@APP.errorhandler(500)
def internal_error(exception):
    return str(exception), 500

@APP.route("/", methods=['POST'])
def get_labels():
    """
    The request should contain 2 form-urlencoded parameters
      1) title : title of the issue
      2) body: body of the issue
    It returns a team/<label> and a component/<label>
    """
    title = request.form.get('title', '')
    body = request.form.get('body', '')
    tokens = tokenize_stem_stop(" ".join([title, body]))
    team_mod = joblib.load(TEAM_FN)
    comp_mod = joblib.load(COMPONENT_FN)
    vec = MY_HASHER.transform([tokens])
    tlabel = team_mod.predict(vec)[0]
    clabel = comp_mod.predict(vec)[0]
    return ",".join([tlabel, clabel])


def tokenize_stem_stop(input_string):
    input_string = input_string.encode('utf-8')
    cur_title_body = TOKENIZER.tokenize(input_string.decode('utf-8').lower())
    return [MY_STEMMER.stem(x) for x in cur_title_body if x not in STOPWORDS]


@APP.route("/update_models", methods=['PUT'])
def update_model(): # pylint: disable=too-many-locals
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

    t_tokens = []
    c_tokens = []
    team_labels = []
    component_labels = []
    for (title, body, label_list) in zip(titles, bodies, labels):
        t_label = [x for x in label_list if x.startswith('team')]
        c_label = [x for x in label_list if x.startswith('component')]
        tokens = tokenize_stem_stop(" ".join([title, body]))
        if t_label:
            team_labels += t_label
            t_tokens += [tokens]
        if c_label:
            component_labels += c_label
            c_tokens += [tokens]
    t_vec = MY_HASHER.transform(t_tokens)
    c_vec = MY_HASHER.transform(c_tokens)

    if team_labels:
        if os.path.isfile(TEAM_FN):
            team_model = joblib.load(TEAM_FN)
            team_model.partial_fit(t_vec, np.array(team_labels))
        else:
            # no team model stored so build a new one
            team_model = SGDClassifier(loss=MY_LOSS, penalty=MY_PENALTY, alpha=MY_ALPHA)
            team_model.fit(t_vec, np.array(team_labels))

    if component_labels:
        if os.path.isfile(COMPONENT_FN):
            component_model = joblib.load(COMPONENT_FN)
            component_model.partial_fit(c_vec, np.array(component_labels))
        else:
            # no comp model stored so build a new one
            component_model = SGDClassifier(loss=MY_LOSS, penalty=MY_PENALTY, alpha=MY_ALPHA)
            component_model.fit(c_vec, np.array(component_labels))

    joblib.dump(team_model, TEAM_FN)
    joblib.dump(component_model, COMPONENT_FN)
    return ""

def configure_logger():
    log_format = '%(asctime)-20s %(levelname)-10s %(message)s'
    file_handler = RotatingFileHandler(LOG_FILE, maxBytes=LOG_SIZE, backupCount=3)
    formatter = logging.Formatter(log_format)
    file_handler.setFormatter(formatter)
    APP.logger.addHandler(file_handler)

if __name__ == "__main__":
    configure_logger()
    APP.run(host="0.0.0.0")
