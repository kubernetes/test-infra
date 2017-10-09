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

import functools
import logging
import os
import re

import cloudstorage as gcs
import jinja2
import webapp2

from google.appengine.api import urlfetch
from google.appengine.api import memcache
from webapp2_extras import sessions

import filters as jinja_filters

JINJA_ENVIRONMENT = jinja2.Environment(
    loader=jinja2.FileSystemLoader(os.path.dirname(__file__) + '/templates'),
    extensions=['jinja2.ext.autoescape', 'jinja2.ext.loopcontrols'],
    trim_blocks=True,
    autoescape=True)
JINJA_ENVIRONMENT.line_statement_prefix = '%'
jinja_filters.register(JINJA_ENVIRONMENT.filters)


class BaseHandler(webapp2.RequestHandler):
    """Base class for Handlers that render Jinja templates."""
    def __init__(self, *args, **kwargs):
        super(BaseHandler, self).__init__(*args, **kwargs)
        # The default deadline of 5 seconds is too aggressive of a target for GCS
        # directory listing operations.
        urlfetch.set_default_fetch_deadline(60)

    # This example code is from:
    # http://webapp2.readthedocs.io/en/latest/api/webapp2_extras/sessions.html
    def dispatch(self):
        # pylint: disable=attribute-defined-outside-init
        # Get a session store for this request.
        self.session_store = sessions.get_store(request=self.request)

        try:
            # Dispatch the request.
            webapp2.RequestHandler.dispatch(self)
        finally:
            # Save all sessions.
            self.session_store.save_sessions(self.response)

    @webapp2.cached_property
    def session(self):
        # Returns a session using the default cookie key.
        return self.session_store.get_session()

    def render(self, template, context):
        """Render a context dictionary using a given template."""
        template = JINJA_ENVIRONMENT.get_template(template)
        self.response.write(template.render(context))


class IndexHandler(BaseHandler):
    """Render the index."""
    def get(self):
        self.render("index.html", {'jobs': self.app.config['jobs']})


def memcache_memoize(prefix, expires=60 * 60, neg_expires=60):
    """Decorate a function to memoize its results using memcache.

    The function must take a single string as input, and return a pickleable
    type.

    Args:
        prefix: A prefix for memcache keys to use for memoization.
        expires: How long to memoized values, in seconds.
        neg_expires: How long to memoize falsey values, in seconds
    Returns:
        A decorator closure to wrap the function.
    """
    # setting the namespace based on the current version prevents different
    # versions from sharing cache values -- meaning there's no need to worry
    # about incompatible old key/value pairs
    namespace = os.environ['CURRENT_VERSION_ID']
    def wrapper(func):
        @functools.wraps(func)
        def wrapped(*args):
            key = '%s%s' % (prefix, args)
            data = memcache.get(key, namespace=namespace)
            if data is not None:
                return data
            else:
                data = func(*args)
                try:
                    if data:
                        memcache.add(key, data, expires, namespace=namespace)
                    else:
                        memcache.add(key, data, neg_expires, namespace=namespace)
                except ValueError:
                    logging.exception('unable to write to memcache')
                return data
        return wrapped
    return wrapper


@memcache_memoize('gs-ls://', expires=60)
def gcs_ls(path):
    """Enumerate files in a GCS directory. Returns a list of FileStats."""
    if path[-1] != '/':
        path += '/'
    return list(gcs.listbucket(path, delimiter='/'))

@memcache_memoize('gs-ls-recursive://', expires=60)
def gcs_ls_recursive(path):
    """Enumerate files in a GCS directory recursively. Returns a list of FileStats."""
    if path[-1] != '/':
        path += '/'
    return list(gcs.listbucket(path))

def pad_numbers(s):
    """Modify a string to make its numbers suitable for natural sorting."""
    return re.sub(r'\d+', lambda m: m.group(0).rjust(16, '0'), s)
