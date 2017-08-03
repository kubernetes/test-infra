"use strict";
var app = angular.module('SubmitQueueModule', ['ngMaterial', 'md.data.table', 'angular-toArrayFilter', 'angularMoment']);

app.config(['$compileProvider', function ($compileProvider) {
  $compileProvider.debugInfoEnabled(false);
}]);

app.controller('SQCntl', ['DataService', '$interval', '$location', SQCntl]);

function SQCntl(dataService, $interval, $location) {
  var self = this;
  self.prs = {};
  self.prsCount = 0;
  self.health = {};
  self.metadata = {};
  self.ciStatus = {};
  self.lastMergeTime = Date();
  self.prQuerySearch = prQuerySearch;
  self.historyQuerySearch = historyQuerySearch;
  self.goToPerson = goToPerson;
  self.selectTab = selectTab;
  self.tabLoaded = {};
  self.functionLoading = {};
  self.queryNum = 0;
  self.selected = 1;
  self.OverallHealth = "";
  self.StatOrder = "-Count";
  self.batch = {};
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
      // NOTE: /e2e was moved to /ci
      // TODO: display a toast noting this
      case "/e2e":
        self.selected=3;	
        self.location.path("/ci");
        break;
      case "/ci":
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

  // Populate data about the submit-queue instance.
  refreshMetadata();

  loadTab(self.selected);

  // Defer loading of data for a tab until it's actually needed.
  // This speeds up the first-boot experience a LOT.
  function loadTab() {
    // reloadFunctions is a map from functions to how often they should run (in minutes)
    var reloadFunctions = {}
    reloadFunctions[refreshPRs] = 10;
    reloadFunctions[refreshGithubE2E] = 0.5;
    reloadFunctions[refreshSQStats] = 30;
    reloadFunctions[refreshHistoryPRs] = 1;
    reloadFunctions[refreshE2EHealth] = 10;
    reloadFunctions[refreshBotStats] = 10;

    // tabFunctionReloads is a map of which tabs need which functions to refresh
    var tabFunctionReloads = {
      0: [refreshPRs],
      1: [refreshGithubE2E, refreshSQStats],
      2: [refreshHistoryPRs],
      3: [refreshE2EHealth, refreshSQStats],
      4: [refreshSQStats, refreshBotStats],
    }
    if (self.tabLoaded[self.selected]) {
      return;
    }
    self.tabLoaded[self.selected] = true;

    var reloadFuncs = tabFunctionReloads[self.selected];
    if (reloadFuncs === undefined)
      return;

    for (var i = 0; i < reloadFuncs.length; i++) {
      var func = reloadFuncs[i];
      if (self.functionLoading[func] == true) {
        continue;
      }
      self.functionLoading[func] = true;

      var updateIntervalMinutes = reloadFunctions[func];
      func();
      $interval(func, updateIntervalMinutes * 60 * 1000);
    }
  }

  // This data is shown in a top banner (when the Queue is blocked),
  // so it's always loaded.
  refreshContinuousTests();
  $interval(refreshContinuousTests, 60000);  // Refresh every minute

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
      self.prsCount = Object.keys(prs).length;
      self.prSearchTerms = getPRSearchTerms();
    }, function errorCallback(response) {
      console.log("Error: Getting SubmitQueue Status");
    });
  }

  function refreshMetadata() {
    dataService.getData('metadata').then(function successCallback(response) {
      var metadata = response.data;
      self.metadata = metadata;
    }, function errorCallback(response) {
      console.log("Error: Getting MetaData for SubmitQueue");
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
      self.batch = response.data.BatchStatus;
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
      self.botStats = response.data;
      for (var key in self.botStats.Analytics) {
        var analytic = self.botStats.Analytics[key];
        analytic.UncachedCount = analytic.Count - analytic.CachedCount;
      }
      self.botStats.UncachedAPICount = self.botStats.APICount - self.botStats.CachedAPICount;
    });
  }

  function refreshE2EHealth() {
    dataService.getData("health").then(function successCallback(response) {
      self.health = response.data;
      if (self.health.TotalLoops !== 0) {
        var percentStable = self.health.NumStable * 100.0 / self.health.TotalLoops;
        self.OverallHealth = Math.round(percentStable) + "%";
      }
    });
  }

  function refreshContinuousTests() {
    dataService.getData('ci-status').then(function successCallback(response) {
      self.ciStatus = processTests(response.data);
    });
  }

  function processTests(data) {
    var results = {};
    angular.forEach(data, function(jobs, key) {
      // job results are keyed by job.Type/category,
      // but in javascript we can't use / in an identifier so use $ instead
      key = key.replace('/', '$');
      results[key] = [];
      var parts = key.split('$');
      var type = parts[0];
      var category = parts[1];
      if (category != "") {
          if (!(category in results)) {
            results[category] = [];
          }
      }
      angular.forEach(jobs, function(job, jobName) {
        var obj = {
          'name': jobName,
          'id': job.build_id,
          'url': job.url,
          'msg': '',
        };
        if (job.state == 'success') {
          // green heavy check mark
          obj.state = '\u2714';
          obj.color = 'green';
        } else {
          //red x mark
          obj.state = '\u2716';
          obj.color = 'red';
        }
        results[key].push(obj);
        if (category != "") {
          results[category].push(obj);
        }
      });
    });
    angular.forEach(results, function(r, key) {
      results[key].sort(function(a, b) {
        if (a.name < b.name) {
          return -1;
        } else if (a.name > b.name) {
          return 1;
        }
        return 0;
      });
    });
    return results;
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


app.filter('loginOrPR', function() {
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

app.filter('refToPRs', function() {
  return function(ref) {
    return ref.replace(/(?:(\d+)|[^:]+):[^,]*,?/g, '$1 ').trim().split(' ');
  }
})

app.service('DataService', ['$http', dataService]);

function dataService($http) {
  return ({
    getData: getData,
  });

  function getData(file) {
    return $http.get(file);
  }
}
