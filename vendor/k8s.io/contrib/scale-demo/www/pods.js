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

var app = angular.module('KubernetesScaleDemo', ['ngMaterial']);

var PodsApp = function(http, scope, q) {
    this.http = http;
    this.scope = scope;
    this.q = q;
};

var image = "gcr.io/google_containers/nginx-scale";

PodsApp.prototype.getServers = function(version) {
    if (!this.servers || !this.servers.items) {
	return 0;
    }
    var img = image + ":" + version;
    var count = 0;
    angular.forEach(this.servers.items, function(pod) {
	    if (pod.spec.containers[0].image == img) {
		count++;
	    }
	});
    return count;
};

PodsApp.prototype.getServersForVersion = function(version) {
    if (!this.servers || !this.servers.items) {
	return [];
    }
    var img = image + ":" + version;
    var result = [];
    angular.forEach(this.servers.items, function(pod) {
	    if (pod.spec.containers[0].image == img) {
		result.push(pod);
	    }
	});
    return result;
};  

PodsApp.prototype.getColor = function(pod) {
    if (pod.status.phase == "Running") {
	return "#4CAF50";
    }
    if (pod.status.phase == "Pending") {
	return "#FFEB3B";
    }
    return "#CC3333";
};

PodsApp.prototype.getVersionColor = function(pod) {
    if (pod.spec.containers[0].image == image + ":0.2") {
	return "#3F51B5";
    }
    if (pod.spec.containers[0].image == image + ":0.3") {
	return "#C5CAE9";
    }
    return "#DDDDDD";
};


// Fetch data from the server and update the data to display
PodsApp.prototype.refresh = function() {
    if (this.refreshInProgress) {
	return;
    }
    this.refreshInProgress = true;
    var promises = [];
    promises.push(this.http.get("/api/v1/pods?labelSelector=run=nginx")
    .success(function(data) {
	    this.servers = data;
	}.bind(this))
    .error(function(data) {
	    console.log("Error!");
	    console.log(data);
	}));

    promises.push(this.http.get("/api/v1/pods?labelSelector=run=vegeta")
    .success(function(data) {
	    this.loadbots = data;
	}.bind(this))
    .error(function(data) {
	    console.log("Error!");
	    console.log(data);
	}));

    this.q.all(promises).then(function() {
	    this.refreshInProgress = false;
	}.bind(this));
};

app.controller('AppCtrl', ['$scope', '$http', '$interval', '$q', function($scope, $http, $interval, $q) {
    $scope.controller = new PodsApp($http, $scope, $q);
    $scope.controller.refresh();

    $interval($scope.controller.refresh.bind($scope.controller), 1000) 
}]);
