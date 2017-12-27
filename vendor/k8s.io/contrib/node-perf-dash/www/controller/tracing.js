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

PerfDashApp.prototype.loadProbes = function() {
    if(this.build == null) {
        this.build = this.minBuild;
    }
    // search for the selected node
    try{
        series = this.allData[this.test].data[this.node][this.build].series;
        dataItem = series[0];
    }
    catch(err){
        console.log(err);
        console.log("Selected build number does not exist.");
        return;
    }
    // merge following dataitems
    for(var i in series) {
        if(i == '0'){
            continue;
        }
        newDataItem = series[i];
        
        if(newDataItem.op_series != null) {
            for(var k in newDataItem.op_series) {
                if(!(k in dataItem.op_series)) {
                    dataItem.op_series[k] = newDataItem.op_series[k];
                }
            }
        }
        if(newDataItem.resource_series != null) {
            for(var k in newDataItem.resource_series) {
                if(!(k in dataItem.resource_series)) {
                    dataItem.resource_series[k] = newDataItem.resource_series[k];
                }
            }
        }   
    }
    this.probes = Object.keys(dataItem.op_series);
}

PerfDashApp.prototype.plotBuildsTracing = function() {
    if(this.probeStart == null || this.probeEnd == null) {
        return;
    }
    latencyPercentiles = {
        'Perc50': [],
        'Perc90': [],
        'Perc99': [],
    }
    this.tracingBuilds = [];
    for (var i in this.builds) { 
        var build = parseInt(this.builds[i]);
        if(build < this.minBuild || build > this.maxBuild) {
            continue;
        }

        startTimeData = this.extractTracingData(this.probeStart, build).sort(function(a, b){return a-b});
        endTimeData = this.extractTracingData(this.probeEnd, build).sort(function(a, b){return a-b});

        latency = arraySubstract(endTimeData, startTimeData).sort(function(a, b){return a-b});

        latencyPercentiles['Perc50'].push(getPercentile(latency, 0.5));
        latencyPercentiles['Perc90'].push(getPercentile(latency, 0.9));
        latencyPercentiles['Perc99'].push(getPercentile(latency, 0.99));

        this.tracingBuilds.push(build);
    }

    this.tracingData = [];
    this.tracingSeries = [];
    for(var metric in latencyPercentiles) {
        this.tracingData.push(latencyPercentiles[metric]);
        this.tracingSeries.push(metric);
    }
    this.tracingOptions = {
        scales: {
            xAxes: [{
                scaleLabel: {
                    display: true,
                    labelString: 'builds',
                }
            }],
            yAxes: [{
                scaleLabel: {
                    display: true,
                    labelString: 'ms',
                }
            }]
        }, 
        elements: {
            line: {
                fill: false,
            },
        },
        legend: {
            display: true,
        },
    };
}

var arraySubstract = function(arr1, arr2) {
    var diff = [];
    var len = Math.min(arr1.length, arr2.length);
    for(i = 0; i < len; i++) {
        diff.push(parseInt(Math.abs(arr1[i] - arr2[i]))/1000000);
    }
    return diff;
}

var getPercentile = function(arr, perc) {
    return arr[Math.floor(arr.length*perc)];
}

PerfDashApp.prototype.extractTracingData = function(probe, build) {
    series = this.allData[this.test].data[this.node][build].series;
    dataItem = series[0];
    if(probe in dataItem.op_series) {
        return dataItem.op_series[probe];
    }

    // try following dataitems
    for(var i in series) {
        if(i == '0'){
            continue;
        }
        newDataItem = series[i];

        if(probe in newDataItem.op_series) {
            return newDataItem.op_series[probe];
        }  
    }
}