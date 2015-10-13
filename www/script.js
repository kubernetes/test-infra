angular.module('SubmitQueueModule', ['ngMaterial']);

angular.module('SubmitQueueModule').controller('SQCntl', ['DataService', SQCntl]);

function SQCntl(dataService) {
  var self = this;
  self.prDisplayValue = "";
  self.prs = {};
  self.users = {};
  self.builds = {};
  self.querySearch = querySearch;
  self.updatePRVisibility = updatePRVisibility;
  self.queryNum = 0;
  // Load all api data
  refresh();

  function refresh() {
    dataService.getData().then(function successCallback(response) {
      var prs = getPRs(response.data.PRStatus);
      __updatePRVisibility(prs);
      self.prs = prs;
      self.prSearchTerms = getPRSearchTerms();
      self.users = getUsers(response.data.UserInfo);
      self.builds = getE2E(response.data.BuildStatus);
      if (response.data.E2ERunning.Number === "") {
        self.e2erunning = [];
      } else {
        self.e2erunning = [ response.data.E2ERunning ];
      }
      self.e2equeue = response.data.E2EQueue;
    }, function errorCallback(response) {
      console.log("Error: Getting SubmitQueue Status");
    });
  }

  function __updatePRVisibility(prs) {
    angular.forEach(prs, function(pr) {
      if (pr.Login.toLowerCase().match(
          "^" + self.prDisplayValue.toLowerCase()) || pr.Num.match("^" + self.prDisplayValue)) {
        pr.show = true;
      } else {
        pr.show = false;
      }
    });
  }

  function updatePRVisibility() {
          __updatePRVisibility(self.prs);
  }

  function getPRs(prs) {
    var result = [];
    angular.forEach(prs, function(value, key) {
      var obj = {
        'Num': key
      };
      angular.forEach(value, function(value, key) {
        obj[key] = value;
      });
      result.push(obj);
    });
    return result;
  }

  function getE2E(builds) {
    var result = [];
    angular.forEach(builds, function(value, key) {
      var obj = {
        'name': key
      };
      if (value == 'Stable') {
        // green check mark
        obj.state = '\u2713';
        obj.color = 'green';
      } else if (value == 'Not Stable') {
        // red X mark
        obj.state = '\u2716';
        obj.color = 'red';
        self.failedBuild = true;
      } else {
        obj.state = 'Error';
        obj.color = 'red';
        obj.msg = value;
        self.failedBuild = true;
      }
      result.push(obj);
    });
    return result;
  }

  function getUsers(users) {
    var result = [];
    angular.forEach(users, function(value, key) {
      var obj = {
        'Login': key
      };
      angular.forEach(value, function(value, key) {
        obj[key] = value;
      });
      result.push(obj);
    });
    return result;
  }

  function searchTermsContain(terms, value) {
    found = false;
    angular.forEach(terms, function(term) {
      if (term.value === value) {
        found = true;
      }
    });
    return found;
  }

  function getPRSearchTerms() {
    var result = [];
    angular.forEach(self.prs, function(pr) {
      llogin = pr.Login.toLowerCase();
      if (!searchTermsContain(result, llogin)) {
        var loginobj = {
          value: llogin,
          display: pr.Login,
        };
        result.push(loginobj);
      }
      if (!searchTermsContain(result, pr.Num)) {
        var numobj = {
          value: pr.Num,
          display: pr.Num,
        };
        result.push(numobj);
      }
    });
    console.log(result);
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

  function querySearch(query) {
    var results = query ? self.prSearchTerms.filter(createFilterFor(query)) : self.prSearchTerms;
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

}

function goToPerson(person) {
  window.location.href = 'https://github.com/' + person;
}

angular.module('SubmitQueueModule').service('DataService', ['$http', dataService]);

function dataService($http) {
  return ({
    getData: getData,
  });

  function getData() {
    return $http.get('api');
  }
}
