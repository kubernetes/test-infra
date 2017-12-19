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

var app = angular.module('KubernetesScaleDemo', ['ngMaterial', 'chart.js'])
    .config(['ChartJsProvider', function (ChartJsProvider) {
		// Configure all charts
		ChartJsProvider.setOptions({
			animation: false
		    });
	    }]);

var limit = 40;

var ScaleApp = function(http, scope, q) {
    this.http = http;
    this.scope = scope;
    this.q = q;

    this.labels = [];
    for (var i = 0; i < limit; i++) {
	this.labels.push(i);
    }

    this.qpsData = [ [] ];
    this.qpsSeries = ['QPS'];
    this.qpsOptions = {
	"scaleOverride" : true,
        "scaleSteps" : 10,
        "scaleStepWidth" : 100000,
        "scaleStartValue" : 0,
	"bezierCurve": false
    }
    this.qpsColors = [{
	"fillColor": 'rgba(232, 127, 12, 0.2)',
	"strokeColor": 'rgba(232, 127, 12, 0.8)',
	"highlightFill": 'rgba(232, 127, 12, 0.8)',
	"highlightStroke": 'rgba(232, 127, 12, 0.8)'
	}];
	    
    this.latencyData = [ [], [] ];
    this.latencySeries = ['Mean', '99th'];
    this.latencyOptions = {
	"scaleOverride" : true,
        "scaleSteps" : 10,
        "scaleStepWidth" : 100,
        "scaleStartValue" : 0,
	"scaleLabel": "<%=value + 'ms'%>"
    }

    this.availData = [ [], [] ];
    this.availSeries = ['Availability', 'Errors' ];
    this.availOptions = {
	"scaleOverride": true,
	"scaleSteps": 10,
	"scaleStepWidth": 10,
	"scaleStartValue": 0,
    };
    this.availColors = [{
	"fillColor": 'rgba(47, 132, 71, 0.2)',
	"strokeColor": 'rgba(47, 132, 71, 0.8)',
	"highlightFill": 'rgba(47, 132, 71, 0.8)',
	"highlightStroke": 'rgba(47, 132, 71, 0.8)'
    },
    {
	"fillColor": 'rgba(132, 47, 71, 0.2)',
	"strokeColor": 'rgba(132, 47, 71, 0.8)',
	"highlightFill": 'rgba(132, 47, 71, 0.8)',
	"highlightStroke": 'rgba(132, 47, 71, 0.8)'
   }];
};

ScaleApp.prototype.onClick = function(data) {
};

// Fetch data from the server and update the data to display
ScaleApp.prototype.refresh = function() {
    if (this.refreshInProgress) {
	return;
    }
    this.refreshInProgress = true;
    var promises = [];
    promises.push(this.http.get("/api/v1/proxy/namespaces/default/services/aggregator/")
    .success(function(data) {
	    this.fullData = data;
	}.bind(this))
    .error(function(data) {
	    console.log("Error!");
	    console.log(data);
	}));

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
    var doneFn = function() { this.refreshInProgress = false; }.bind(this);
    this.q.all(promises).then(doneFn, doneFn);
};

ScaleApp.prototype.getServerCount = function() {
    if (!this.servers || !this.servers.items) {
	return 0;
    }
    var count = 0;
    angular.forEach(this.servers.items, function(pod) {
	    if (pod.status.phase == "Running") {
		count++;
	    }
	});
    return count;
};

ScaleApp.prototype.getLoadbotCount = function() {
    if (!this.loadbots || !this.loadbots.items) {
	return 0;
    }
    var count = 0;
    angular.forEach(this.loadbots.items, function(pod) {
	    if (pod.status.phase == "Running") {
		count++;
	    }
	});
    return count;
};

ScaleApp.prototype.getLoadbotReports = function() {
    if (!this.loadbots) {
	return nil
    }
    var promises = []
    for (var i = 0; i < this.loadbots.items.length; i++) {
	var pod = this.loadbots.items[i];
	var promise = this.http.get("/api/v1/proxy/namespaces/default/pods/" + pod.metadata.name + ":8080/")
	.success(function(data) {
		this.loadData = data;
	    }.bind(pod))
	.error(function(data) {
		console.log("Error loading loadbot");
		console.log(data);
	    });
	promises.push(promise)
    }

    this.q.all(promises).then(function() {
	    this.updateGraphData();
	}.bind(this));

};

ScaleApp.prototype.slideWindow = function(data, newPoint) {
    if (data.length < limit) {
	data.push(newPoint);
	return data;
    }
    var newData = [];
    for (var i = 0; i < limit; i++) {
	newData[i] = data[i + 1];
    }
    newData[limit - 1] = newPoint;

    return newData;
}

ScaleApp.prototype.updateGraphData = function() {
    var qps = this.getQPS();
    var latency = this.getLatency();
    var success = this.getSuccess();

    this.qpsData[0] = this.slideWindow(this.qpsData[0], qps);
    this.latencyData[0] = this.slideWindow(this.latencyData[0], latency["mean"]);
    this.latencyData[1] = this.slideWindow(this.latencyData[1], latency["99th"]);
    this.availData[0] = this.slideWindow(this.availData[0], success);
    this.availData[1] = this.slideWindow(this.availData[1], 100 - success);
};

ScaleApp.prototype.getQPS = function() {
    if (!this.fullData) {
	return 0;
    }
    var qps = 0;
    angular.forEach(this.fullData, function(value) {
	    if (value && value.rate) {
		qps += value.rate;
	    }
	});
    return qps;
};

ScaleApp.prototype.getSuccess = function() {
    if (!this.fullData) {
	return 0;
    }
    var success = 0;
    var count = 0;
    angular.forEach(this.fullData, function(value) {
	    if (value && value.success) {
		success += value.success * 100;
		count++;
	    }
	});
    return success / count;
};

ScaleApp.prototype.getLatency = function() {
    if (!this.fullData) {
	return {};
    }
    var latency = {
	"mean": 0,
	"99th": 0
    };
    var count = 0;
    angular.forEach(this.fullData, function(datum) {
	    if (datum.latencies) {
		latency.mean += datum.latencies.mean / 1000000;
		latency["99th"] += datum.latencies["99th"] / 1000000;
		count++;
	    }
	});
    if (count == 0) {
	return {};
    }
    latency.mean = (latency.mean/count);
    latency["99th"] = (latency["99th"]/count);

    return latency;
};

ScaleApp.prototype.getNumServers = function() {
    return this.getNumPods(this.servers);
};

ScaleApp.prototype.getNumLoadbots = function() {
    return this.getNumPods(this.loadbots);
};

ScaleApp.prototype.getNumPods = function(pods) {
    if (pods) {
	return pods.items.length;
    }
    return 0;
};


app.controller('AppCtrl', ['$scope', '$http', '$interval', '$q', function($scope, $http, $interval, $q) {
    $scope.controller = new ScaleApp($http, $scope, $q);
    $scope.controller.refresh();

    $interval($scope.controller.refresh.bind($scope.controller), 1000) 
    $interval($scope.controller.updateGraphData.bind($scope.controller), 1000) 
}]);
