"use strict";
angular.module('CherrypickModule', ['ngMaterial', 'md.data.table', 'angular-toArrayFilter']);

angular.module('CherrypickModule').controller('CPCntl', ['DataService', '$interval', '$location', CPCntl]);

function CPCntl(dataService, $interval, $location) {
  var self = this;
  self.queue = {};
  self.raw = {};
  self.selectTab = selectTab;
  self.selected = 0;
  self.StatOrder = "-Count";
  self.location = $location;

  var path = $location.path();
  if (path.length > 0) {
      switch (path) {
      case "/queue":
	  self.selected=0;
	  break;
      case "/raw":
	  self.selected=1;
	  break;
      case "/info":
	  self.selected=2;
	  break;
      default:
	  console.log("unknown path: " + path);
	  break;
      }
   }

  // Refresh data every minute
  refreshQueue();
  $interval(refreshQueue, 60000);

  function refreshQueue() {
    dataService.getData('queue').then(function successCallback(response) {
      self.queueData = response.data;
    }, function errorCallback(response) {
      console.log("Error: Getting Cherrypick Status");
    });
   }

  // Refresh data every minute
  refreshRaw();

  function refreshRaw() {
    dataService.getData('raw').then(function successCallback(response) {
      self.raw = response.data;
    }, function errorCallback(response) {
      console.log("Error: Getting Cherrypick Raw Status");
    });
   }

  // Refresh every 10 minutes
  refreshStats();
  $interval(refreshStats, 600000);

  function refreshStats() {
    dataService.getData('stats').then(function successCallback(response) {
      var nextLoop = new Date(response.data.NextLoopTime);
      document.getElementById("next-run-time").innerHTML = nextLoop.toLocaleTimeString();

      self.botStats = response.data.Analytics;
      self.APICount = response.data.APICount;
      self.CachedAPICount = response.data.CachedAPICount;
      document.getElementById("api-calls-per-sec").innerHTML = response.data.APIPerSec;
      document.getElementById("github-api-limit-count").innerHTML = response.data.LimitRemaining;
      var nextReset = new Date(response.data.LimitResetTime);
      document.getElementById("github-api-limit-reset").innerHTML = nextReset.toLocaleTimeString();
    });
  }

  function selectTab(tabName) {
    self.location.path('/' + tabName);
  }

  getQueueInfo()

  function getQueueInfo() {
    dataService.getData('queue-info').then(function successCallback(response) {
      document.getElementById("queue-info").innerHTML = response.data;
    });
  }
}


angular.module('CherrypickModule').service('DataService', ['$http', dataService]);

function dataService($http) {
  return ({
    getData: getData,
  });

  function getData(file) {
    return $http.get(file);
  }
}
