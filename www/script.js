var app = angular.module('SubmitQueueApp', ['ngMaterial']);

var SubmitQueueApp = function(http, scope) {
    this.http = http;
    this.scope = scope;
};

SubmitQueueApp.prototype.refresh = function() {
    this.http.get("/api")
    .success(function(data) {
	    this.data = data;
	    this.data.Message.reverse();
	    this.builds = this.getBuilds();
	    console.log(this.builds);
	}.bind(this))
    .error(function(data) {
            console.log("error fetching api");
	    console.log(data);
	});
};

SubmitQueueApp.prototype.getBuilds = function() {
    var result = [];
    angular.forEach(this.data.BuildStatus, function(value, key) {
	    var obj = {'name': key};
	    if (value == 'Stable') {
		// green check mark
		obj['state'] = '\u2713';
		obj['color'] = 'green'
	    } else if (value == 'Not Stable') {
		// red X mark
		obj['state'] = '\u2716';
		obj['color'] = 'red';
	    } else {
		obj['state'] = '?';
		obj['color'] = 'black';
	    }
	    result.push(obj)
	});
    return result;
};
    

app.controller('AppCtrl', ['$scope', '$http', '$interval', function($scope, $http, $interval) {
    $scope.controller = new SubmitQueueApp($http, $scope);
    $scope.controller.refresh();
    $interval($scope.controller.refresh.bind($scope.controller), 2500) 
}]);
