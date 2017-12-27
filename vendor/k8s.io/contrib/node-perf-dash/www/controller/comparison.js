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

var testNodeSeparator = '//'

// addToComparison adds current test/node into comparison list
PerfDashApp.prototype.addToComparison = function() {
    if(!this.test | !this.node) {
        return;
    }
    id = this.test + testNodeSeparator + this.node;
    this.comparisonList.push({
        id: id,
        test: this.test,
        node: this.node,
        detail: this.testInfo.info[this.test],
    });

    this.comparisonListSelected.push(id);
}

// plotComparison plots comparison
PerfDashApp.prototype.plotComparison = function() {
    // select and aggregate data
    this.selectTestData();
    // plot charts
    this.plotComparisonChart(aggDataMap);
};

PerfDashApp.prototype.selectTestData = function() {
    tests = [];
    nodesPerTest = {};
    
    angular.forEach(this.comparisonListSelected, function(id) {
        test = id.split(testNodeSeparator)[0];
        node = id.split(testNodeSeparator)[1];

        if(tests.indexOf(test) == -1) {
            tests.push(test);
            nodesPerTest[test] = [];
        }
        nodesPerTest[test].push(node);
    });

    aggDataMap = {}
    angular.forEach(this.allData, function(testData, testName) {
        if(tests.indexOf(testName) > -1) {
            angular.forEach(nodesPerTest[testName], function(node) {
                aggregateBuilds = this.aggregateBuild(testData.data[node]);
                if(Object.keys(aggregateBuilds).length > 0) {
                    aggDataMap[testName + testNodeSeparator + node] = aggregateBuilds;
                }
            }, this);
        }
    }, this)    

    //console.log(JSON.stringify(aggDataMap))
    return aggDataMap;
};

// aggregate a series of builds to one for comparing different configurations
PerfDashApp.prototype.aggregateBuild = function(builds) {
    selectedPerBuild = {};
    
    angular.forEach(builds, function(build, name) {
        if(build.perf.length > 0 && parseInt(name) >= this.minBuild && parseInt(name) <= this.maxBuild) {
            selectedPerBuild[name] = build.perf;
        }
    }, this)

    if(Object.keys(selectedPerBuild).length == 0) { 
        return {};
    } else {
        switch(this.aggregateType) {
        case 'latest':
            return selectedPerBuild[Math.max.apply(null, Object.keys(selectedPerBuild))];
        case 'average':
            avgItem = {}
            first = true
            
            angular.forEach(selectedPerBuild, function(item, ignore) {
                if(first) {
                    avgItem = item
                    first = false
                } else {
                    angular.forEach(item.data, function(value, metric) {
                        avgItem.data[metric] += value 
                    })
                    
                }
            })
            angular.forEach(avgItem.data, function(ignore, metric) {
                avgItem.data[metric] /= selectedPerBuild.length
            })
            return avgItem
        } 
    }
};

// Update the data to graph, using selected labels
PerfDashApp.prototype.plotComparisonChart = function(aggDataMap) {
    // get data for each plot
    angular.forEach(plots, function(plot){
        plotRule = plotRules[plot];
        this.comparisonSeriesDataMap[plot] = [];
        this.comparisonSeriesMap[plot] = [];
        switch(plot) {
            case 'latency':
            case 'throughput':
            case 'kubelet_cpu':
            case 'kubelet_memory':
            case 'runtime_cpu':
            case 'runtime_memory':
                selectedLabels = plotRule.labels;
                break;
            default:
                console.log('unkown plot type ' + plot)
                return;              
        }
        result = this.getComparisonData(aggDataMap, selectedLabels);
        this.comparisonLabelsMap[plot] = JSON.parse(JSON.stringify(plotRule.metrics));

        if (Object.keys(result).length <= 0) {
            return;
        }

        this.comparisonOptionsMap[plot]= {
            scales: {
                yAxes: [{
                    scaleLabel: {
                        display: true,
                        labelString: result[Object.keys(result)[0]].unit,
                    }
                }]
            },  
            legend: {
                display: true,
            },
        };

        angular.forEach(this.comparisonListSelected, function(id) {
            item = result[id]
            this.comparisonSeriesDataMap[plot].push(this.getComparisonStream(item, plotRule.metrics));
            this.comparisonSeriesMap[plot].push(id);
        }, this);

    }, this)
};

// Extract a time series of data for specific labels
PerfDashApp.prototype.getComparisonData = function(aggDataMap, labels) {
    var result = {};

    angular.forEach(aggDataMap, function(items, id) {
        angular.forEach(items, function(item) {
            var match = true;
            angular.forEach(labels, function(label, name) {
                if (item.labels[name] != label) {
                    match = false;
                }
            });
            if (match) {
                result[id] = item;
            }
        });
    });
    return result;
};

// Given a slice of data, turn it into a time series of numbers
// 'data' is an array of APICallLatency objects
// 'stream' is a selector for latency data, (e.g. 'Perc50')
PerfDashApp.prototype.getComparisonStream = function(item, rule) {
    var result = [];
    angular.forEach(rule, function(metric) {
        result.push(item.data[metric]);
    });
    return result;
};

PerfDashApp.prototype.deleteSelectedTest = function(item, list) {
    deleteFromList(item, this.comparisonList);
    deleteFromList(item.id, this.comparisonListSelected);
};

// for selecting tests in comparison list
PerfDashApp.prototype.exists = function (item, list) {
    return list.indexOf(item) > -1;
};

PerfDashApp.prototype.toggle = function (item, list) {
    var idx = list.indexOf(item);
    if (idx > -1) {
      list.splice(idx, 1);
    }
    else {
      list.push(item);
    }
    //console.log(JSON.stringify(list));
    //this.comparisonChanged()
};

var deleteFromList = function(item, list) {
    var idx = list.indexOf(item);
    if (idx > -1) {
      list.splice(idx, 1);
    }
};