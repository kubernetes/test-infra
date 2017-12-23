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

PerfDashApp.prototype.buildChanged = function() {
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

        // TODO(coufon): be aware that duplicated data may occur when test fails
        // because the error output also contains test result.
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

    this.timeseries = dataItem.resource_series;
    this.latencySeriesMap = dataItem.op_series;

    this.plotTimeSeries();
}

colorList = ['rgba(51,153,255,0.3)', 'rgba(0,204,102,0.3)', 
             'rgba(0,255,255,0.3)', 'rgba(102,0,102,0.3)', 
             'rgba(255,0,0,0.3)', 'rgba(255,128,0,0.3)',
             'rgba(204,204,0,0.3)', 'rgba(0,0,153,0.3)',
             'rgba(153,0,0,0.3)'];

opTimeSeriesShowList = ['create', 'running', 'pod_running', 'pod_config_change'];

// Plot the time series data for the selected build
PerfDashApp.prototype.plotTimeSeries = function() {
    // align timeline
    var start = Math.min(this.timeseries['kubelet']['ts'][0],
        this.timeseries['runtime']['ts'][0])

    // get data for each plot
    angular.forEach(seriesPlots, function(plot){
        var ctx, dataSets;

        switch(plot) {
            case 'latency':
                dataSets = [];
                var i = 0;
                
                for(var key in this.latencySeriesMap) {
                    let hidden  = true;
                    for(var j in opTimeSeriesShowList) {
                        if(key == opTimeSeriesShowList[j]) {
                            hidden = false;
                        }
                    }

                    dataSets.push({ 
                        label: key,
                        data: getAccumSeries(this.latencySeriesMap[key].map(function(value){
                            return ((value - start)/1e9).toFixed(1);
                        })),
                        backgroundColor: colorList[i++],
                        hidden: hidden,
                    })
                }
                unit = "#Pod"
                ctx = document.getElementById("series_latency").getContext("2d");
                break;
            case 'kubelet_cpu':
                dataSets = [{ 
                    label: 'resource',
                    data: combineSeries(
                        this.timeseries['kubelet']['ts'].map(function(value){
                            return ((value - start)/1e9).toFixed(1);
                        }), 
                        this.timeseries['kubelet']['cpu']
                    ),
                }];
                unit = this.timeseries['kubelet']['unit']['cpu']
                ctx = document.getElementById("series_kubelet_cpu").getContext("2d");
                break;
            case 'kubelet_memory':
                dataSets = [{ 
                    label: 'resource',
                    data: combineSeries(
                        this.timeseries['kubelet']['ts'].map(function(value){
                            return ((value - start)/1e9).toFixed(1);
                        }), 
                        this.timeseries['kubelet']['memory']
                    ),
                }];
                unit = this.timeseries['kubelet']['unit']['memory']
                ctx = document.getElementById("series_kubelet_memory").getContext("2d");
                break;
            case 'runtime_cpu':
                dataSets = [{ 
                    label: 'resource',
                    data: combineSeries(
                        this.timeseries['runtime']['ts'].map(function(value){
                            return ((value - start)/1e9).toFixed(1);
                        }), 
                        this.timeseries['runtime']['cpu']
                    ),
                }];
                unit = this.timeseries['runtime']['unit']['cpu'];
                ctx = document.getElementById("series_runtime_cpu").getContext("2d");
                break;
            case 'runtime_memory':
                dataSets = [{ 
                    label: 'resource',
                    data: combineSeries(
                        this.timeseries['runtime']['ts'].map(function(value){
                            return ((value - start)/1e9).toFixed(1);
                        }), 
                        this.timeseries['runtime']['memory']
                    ),
                }];
                unit = this.timeseries['runtime']['unit']['memory'];
                ctx = document.getElementById("series_runtime_memory").getContext("2d");
                break;
            default:
                console.log('unkown plot type ' + plot);
                return;              
        }

        if(this.clearSeriesCharts) {
            this.seriesCharts = {};
            this.clearSeriesCharts = false;
        }

        if(plot in this.seriesCharts) {
            //console.log("update")
            this.seriesCharts[plot].data.datasets = dataSets;
            this.seriesCharts[plot].update();
        } else {
            //console.log("new")
            this.seriesCharts[plot] = new Chart(ctx, {
                type: 'line',
                data: {
                    datasets: dataSets,
                },
                options: {
                    scales: {
                        xAxes: [{
                            type: 'linear',
                            position: 'bottom',
                            scaleLabel: {
                                display: true,
                                labelString: 'time(s)',
                            }
                        }],
                        yAxes: [{
                            scaleLabel: {
                                display: true,
                                labelString: unit,
                            }
                        }],
                    },
                    legend: {
                        display: (plot == 'latency')?true:false,
                    }
                }
            });
        } 
    }, this)
};

// combineSeries combines two arraies into one array of tuples (x:v1, y:v2)
var combineSeries = function(s0, s1) {
    if(s0.length != s1.length) {
        console.log("Series length mismatch.");
        return;
    }
    var ret = [];
    for(var i in s0) {
        ret.push({
            x: s0[i],
            y: s1[i],
        })
    }
    return ret
}

// getAccumSeries calculate accumulated time series data
var getAccumSeries = function(s0) {
    var ret = [];
    var sum = 0;
    for(var i in s0) {
        ret.push({
            x: s0[i],
            y: ++sum,
        });
    }
    return ret    
}