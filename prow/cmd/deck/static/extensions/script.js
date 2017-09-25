// This file serves for custom javascript extensions

document.addEventListener("DOMContentLoaded", function(event) { 
	var head = document.getElementsByTagName('head')[0];
	head.insertAdjacentHTML('beforeend', '<link rel="shortcut icon" href="extensions/logo.png">');

	var header = document.getElementsByTagName('header')[0];
	header.insertAdjacentHTML('afterbegin', '<a class="logo" ng-href="https://github.com/openshift/origin"><img src="extensions/logo.png" alt="openshfit logo" class="titleLogo"></a>');
});
