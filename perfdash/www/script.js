/*
Copyright 2015 The Kubernetes Authors All rights reserved.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

var app = angular.module('PerfDashApp', ['ngMaterial', 'chart.js']);

var PerfDashApp = function(http, scope) {
    this.http = http;
    this.scope = scope;
    this.testNames = [];
    this.onClick = this.onClickInternal_.bind(this);
    this.cap = 0;
};


PerfDashApp.prototype.onClickInternal_ = function(data, evt, chart) {
    console.log(this, data, evt, chart);
    if (evt.ctrlKey) {
      this.cap = (chart.scale.min + chart.scale.max) / 2;
      this.labelChanged();
      return;
    }

    // Get location
    // TODO(random-liu): Make the URL configurable if we want to support more
    // buckets in the future.
    window.open("https://k8s-gubernator.appspot.com/build/kubernetes-jenkins/logs/" + this.job + "/" + data[0].label + "/", "_blank");
};

// Fetch data from the server and update the data to display
PerfDashApp.prototype.refresh = function() {
    this.http.get("api")
            .success(function(data) {
                this.testNames = Object.keys(data);
                this.testName = this.testNames[0];
                this.allData = data;
                this.testNameChanged();
            }.bind(this))
    .error(function(data) {
        console.log("error fetching result");
        console.log(data);
    });
};

// Update the data to graph, using selected labels
PerfDashApp.prototype.labelChanged = function() {
    this.seriesData = [];
    this.series = [];
    result = this.getData(this.selectedLabels);
    if (result.length <= 0) {
        return;
    }
    // All the unit should be the same
    this.options = {scaleLabel: "<%=value%> "+result[0].unit};
    // Start with higher percentiles, since their values are usually strictly higher
    // than lower percentiles, which avoids obscuring graph data. It also orders data
    // in the onHover labels more naturally.
    var seriesLabels = Object.keys(result[0].data);
    seriesLabels.sort();
    seriesLabels.reverse();
    angular.forEach(seriesLabels, function(name) {
        this.seriesData.push(this.getStream(result, name));
        this.series.push(name);
    }, this);
    this.cap = 0;
};

// Update the data to graph, using the selected testName
PerfDashApp.prototype.testNameChanged = function() {
    this.data = this.allData[this.testName].builds;
    this.job = this.allData[this.testName].job;
    this.builds = this.getBuilds();
    this.labels = this.getLabels();
    this.labelChanged();
};

// Get all of the builds for the data set (e.g. build numbers)
PerfDashApp.prototype.getBuilds = function() {
    return Object.keys(this.data)
};

// Get the set of all labels (e.g. 'resources', 'verbs') in the data set
PerfDashApp.prototype.getLabels = function() {
    var set = {};
    angular.forEach(this.data, function(items, build) {
        angular.forEach(items, function(item) {
            angular.forEach(item.labels, function(label, name) {
                if (set[name] == undefined) {
                    set[name] = {}
                }
                set[name][label] = true
            });
        });
    });

    this.selectedLabels = {}
    var labels = {};
    angular.forEach(set, function(items, name) {
        labels[name] = [];
        angular.forEach(items, function(ignore, item) {
            if (this.selectedLabels[name] == undefined) {
              this.selectedLabels[name] = item;
            }
            labels[name].push(item)
        }, this);
    }, this);
    return labels;
};

// Extract a time series of data for specific labels
PerfDashApp.prototype.getData = function(labels) {
    var result = [];
    angular.forEach(this.data, function(items, build) {
        angular.forEach(items, function(item) {
            var match = true;
            angular.forEach(labels, function(label, name) {
                if (item.labels[name] != label) {
                    match = false;
                }
            });
            if (match) {
                result.push(item);
            }
        });
    });
    return result;
};

// Given a slice of data, turn it into a time series of numbers
// 'data' is an array of APICallLatency objects
// 'stream' is a selector for latency data, (e.g. 'Perc50')
PerfDashApp.prototype.getStream = function(data, stream) {
    var result = [];
    angular.forEach(data, function(value) {
        var x = value.data[stream];
        if (this.cap != 0 && x > this.cap) {
          x = this.cap;
        }
        result.push(x);
    }, this);
    return result;
};

app.controller('AppCtrl', ['$scope', '$http', '$interval', function($scope, $http, $interval) {
    $scope.controller = new PerfDashApp($http, $scope);
    $scope.controller.refresh();

    // Refresh every 5 min.  The data only refreshes every 10 minutes on the server
    $interval($scope.controller.refresh.bind($scope.controller), 300000)
}]);
