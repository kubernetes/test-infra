FROM ruby:2.2

WORKDIR /srv/jekyll
ADD Gemfile .
RUN gem install bundler
RUN bundle install
