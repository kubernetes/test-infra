"use strict";
angular.module('SubmitQueueModule', ['ngMaterial', 'md.data.table', 'angular-toArrayFilter', 'angularMoment']);

angular.module('SubmitQueueModule').controller('SQCntl', ['DataService', '$interval', '$location', SQCntl]);

function SQCntl(dataService, $interval, $location) {
  var self = this;
  self.prs = {};
  self.users = {};
  self.builds = {};
  self.health = {};
  self.lastMergeTime = Date();
  self.prQuerySearch = prQuerySearch;
  self.historyQuerySearch = historyQuerySearch;
  self.goToPerson = goToPerson;
  self.selectTab = selectTab;
  self.tabLoaded = {};
  self.queryNum = 0;
  self.selected = 1;
  self.OverallHealth = "";
  self.StartTime = "";
  self.StatOrder = "-Count";
  self.location = $location;

  // http://submit-queue.k8s.io/#?prDisplay=eparis&historyDisplay=15999
  //  will show all PRs opened by eparis and all historic decisions for PR #15999
  var vars = $location.search();
  self.prDisplayValue = vars.prDisplay;
  self.historyDisplayValue = vars.historyDisplay;

  var path = $location.path();
  if (path.length > 0) {
      switch (path) {
      case "/prs":
          self.selected=0;
          break;
      case "/queue":
          self.selected=1;
          break;
      case "/history":
          self.selected=2;
          break;
      case "/e2e":
          self.selected=3;
          break;
      case "/info":
          self.selected=4;
          break;
      default:
          console.log("unknown path: " + path);
          break;
      }
  }

  loadTab(self.selected);

  // Defer loading of data for a tab until it's actually needed.
  // This speeds up the first-boot experience a LOT.
  function loadTab() {
    // tab: [reloading-function, update interval in minutes, ...] (can repeat)
    var tabFunctionReloads = {
      0: [refreshPRs, 10],
      1: [refreshGithubE2E, 0.5, refreshSQStats, 30],
      2: [refreshHistoryPRs, 1],
      3: [refreshE2EHealth, 10],
      4: [refreshUsers, 15, refreshBotStats, 10],
    }
    if (self.tabLoaded[self.selected])
      return;
    var tabReload = tabFunctionReloads[self.selected];
    if (tabReload !== undefined) {
      for (var i = 0; i < tabReload.length; i += 2) {
        var func = tabReload[i];
        var updateIntervalMinutes = tabReload[i + 1];
        func();
        $interval(func, updateIntervalMinutes * 60 * 1000);
      }
      self.tabLoaded[self.selected] = true;
    }
  }

  // This data is shown in a top banner (when the Queue is blocked),
  // so it's always loaded.
  refreshGoogleInternalCI();
  $interval(refreshGoogleInternalCI, 60000);  // Refresh every minute

  // Request Avatars that are only as large necessary (CSS limits them to 40px)
  function fixPRAvatars(prs) {
    angular.forEach(prs, function(pr) {
      if (/^https:\/\/avatars.githubusercontent.com\/u\/\d+\?v=3$/.test(pr.AvatarURL)) {
        pr.AvatarURL += '&size=40';
      }
    });
  }

  function refreshPRs() {
    dataService.getData('prs').then(function successCallback(response) {
      var prs = response.data.PRStatus;
      fixPRAvatars(prs);
      self.prs = prs;
      self.prSearchTerms = getPRSearchTerms();
    }, function errorCallback(response) {
      console.log("Error: Getting SubmitQueue Status");
    });
  }

  function refreshGithubE2E() {
    dataService.getData('github-e2e-queue').then(function successCallback(response) {
      if (response.data.E2ERunning.Number == 0) {
        self.e2erunning = [];
      } else {
        self.e2erunning = [response.data.E2ERunning];
      }
      self.e2equeue = response.data.E2EQueue;
      fixPRAvatars(self.e2equeue);
    });
  }

  function refreshHistoryPRs() {
    dataService.getData('history').then(function successCallback(response) {
      var prs = response.data;
      fixPRAvatars(prs);
      self.historyPRs = prs;
      self.historySearchTerms = getHistorySearchTerms();
    });
  }

  function refreshSQStats() {
    dataService.getData('sq-stats').then(function successCallback(response) {
      self.sqStats = response.data;
      self.lastMergeTime = new Date(response.data.LastMergeTime)
    });
  }

  function refreshBotStats() {
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

  function refreshUsers() {
    dataService.getData('users').then(function successCallback(response) {
      self.users = response.data;
    });
  }

  function refreshE2EHealth() {
    dataService.getData("health").then(function successCallback(response) {
      self.health = response.data;
      if (self.health.TotalLoops !== 0) {
        var percentStable = self.health.NumStable * 100.0 / self.health.TotalLoops;
        self.OverallHealth = Math.round(percentStable) + "%";
        self.StartTime = new Date(self.health.StartTime).toLocaleString();
      }
      updateBuildStability(self.builds, self.health);
    });
  }

  function refreshGoogleInternalCI() {
    dataService.getData('google-internal-ci').then(function successCallback(response) {
      var result = getE2E(response.data);
      self.builds = result.builds;
      updateBuildStability(self.builds, self.health);
      self.failedBuild = result.failedBuild;
    });
  }

  function getE2E(builds) {
    var result = [];
    var failedBuild = false;
    angular.forEach(builds, function(job, key) {
      var obj = {
        'name': key,
        'id': job.ID,
      };
      if (job.Status == 'Stable') {
        // green check mark
        obj.state = '\u2713';
        obj.color = 'green';
      } else if (job.Status == 'Not Stable') {
        // red X mark
        obj.state = '\u2716';
        obj.color = 'red';
        failedBuild = true;
      } else {
        obj.state = 'Error';
        obj.color = 'red';
        obj.msg = job.Status;
        failedBuild = true;
      }
      obj.stability = '';
      result.push(obj);
    });
    return {
      builds: result,
      failedBuild: failedBuild,
    };
  }

  function updateBuildStability(builds, health) {
    if (Object.keys(builds).length === 0 ||
        health.TotalLoops === 0 || health.NumStablePerJob === undefined) {
      return;
    }
    angular.forEach(builds, function(build) {
      var key = build.name;
      if (key in self.health.NumStablePerJob) {
        var percentStable = health.NumStablePerJob[key] * 100.0 / health.TotalLoops;
        build.stability = Math.round(percentStable) + "%"
      }
    });
  }

  function searchTermsContain(terms, value) {
    var found = false;
    angular.forEach(terms, function(term) {
      if (term.value === value) {
        found = true;
      }
    });
    return found;
  }

  function getPRSearchTerms() {
    return getSearchTerms(self.prs);
  }

  function getHistorySearchTerms() {
    return getSearchTerms(self.historyPRs);
  }

  function getSearchTerms(prs) {
    var result = [];
    angular.forEach(prs, function(pr) {
      var llogin = pr.Login.toLowerCase();
      if (!searchTermsContain(result, llogin)) {
        var loginobj = {
          value: llogin,
          display: pr.Login,
        };
        result.push(loginobj);
      }
      if (!searchTermsContain(result, pr.Num)) {
        var numobj = {
          value: pr.Number.toString(),
          display: pr.Number,
        };
        result.push(numobj);
      }
    });
    result.sort(compareSearchTerms);
    return result;
  }

  /* We need to compare the 'value' to get a sane sort */
  function compareSearchTerms(a, b) {
    if (a.value > b.value) {
      return 1;
    }
    if (a.value < b.value) {
      return -1;
    }
    return 0;
  }

  function prQuerySearch(query) {
    return querySearch(query, self.prSearchTerms);
  }

  function historyQuerySearch(query) {
    return querySearch(query, self.historySearchTerms);
  }

  function querySearch(query, terms) {
    var results = query ? terms.filter(createFilterFor(query)) : terms;
    return results;
  }

  /**
   * Create filter function for a query string
   */
  function createFilterFor(query) {
    var lowercaseQuery = angular.lowercase(query);
    return function filterFn(state) {
      return (state.value.indexOf(lowercaseQuery) === 0);
    };
  }

  function goToPerson(person) {
    console.log(person);
    window.location.href = 'https://github.com/' + person;
  }

  function selectTab(tabName) {
    self.location.path('/' + tabName);
    loadTab();
  }

  getPriorityInfo()

  function getPriorityInfo() {
    dataService.getData('priority-info').then(function successCallback(response) {
      document.getElementById("priority-info").innerHTML = response.data;
    });
  }

  getMergeInfo()

  function getMergeInfo() {
    dataService.getData('merge-info').then(function successCallback(response) {
      document.getElementById("merge-info").innerHTML = response.data;
    });
  }
}


angular.module('SubmitQueueModule').filter('loginOrPR', function() {
  return function(prs, searchVal) {
    searchVal = searchVal || "";
    prs = prs || [];
    searchVal = angular.lowercase(searchVal);
    var out = [];

    angular.forEach(prs, function(pr) {
      var shouldPush = false;
      var llogin = pr.Login.toLowerCase();
      if (llogin.indexOf(searchVal) === 0) {
        shouldPush = true;
      }
      if (pr.Number.toString().indexOf(searchVal) === 0) {
        shouldPush = true;
      }
      if (shouldPush) {
        out.push(pr);
      }
    });
    return out;
  };
});

angular.module('SubmitQueueModule').service('DataService', ['$http', dataService]);

function dataService($http) {
  return ({
    getData: getData,
  });

  function getData(file) {
    return $http.get(file);
  }
}
