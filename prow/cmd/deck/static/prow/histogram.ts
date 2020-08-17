import {ProwJobState} from "../api/prow";

export class JobHistogram {
  public start: number;
  public end: number;
  private data: JobSample[];

  constructor() {
      this.data = [];
      this.start = Number.MAX_SAFE_INTEGER;
      this.end = 0;
  }
  // add adds a sample to the histogram, filtering states that didn't result in success or clear
  // failure, and updating the range of the histogram data.
  public add(sample: JobSample) {
      if (!(sample.state === "success" || sample.state === "failure" || sample.state === "error")) {
          return;
      }
      if (sample.start < this.start) {
          this.start = sample.start;
      }
      if (sample.start > this.end) {
          this.end = sample.start;
      }
      this.data.push(sample);
  }
  // buckets assigns all samples between start and end into cols buckets, sorted by
  // start timestamp, while the buckets themselves are sorted by duration.
  public buckets(start: number, end: number, cols: number): JobBuckets {
      this.data.sort((s1, s2) => s1.start - s2.start);

      const buckets: JobSample[][] = [[]];
      const stride = (end - start) / cols;
      let next = start + stride;
      let max = 0;
      this.data.forEach((sample) => {
          if (sample.start < start || sample.start > end) {
              return;
          }
          if (sample.duration > max) {
              max = sample.duration;
          }
          if (sample.start < next || sample.start === end) {
              buckets[buckets.length - 1].push(sample);
              return;
          }

          const bucket = buckets[buckets.length - 1];
          bucket.sort((s1, s2) => s1.duration - s2.duration);

          next = next + stride;
          while (next < sample.start) {
              buckets.push([]);
              next = next + stride;
          }
          buckets.push([sample]);
      });
      if (buckets.length > 0) {
          const lastBucket = buckets[buckets.length - 1];
          lastBucket.sort((s1, s2) => s1.duration - s2.duration);
      }
      while (buckets.length < cols) {
          buckets.push([]);
      }
      return new JobBuckets(buckets, start, end, max);
  }
  // length returns the number of samples in the histogram.
  public get length(): number {
      return this.data.length;
  }
}

export class JobSample {
  constructor(public start: number,
              public duration: number,
              public state: ProwJobState,
              public row: number) {}
}

export class JobBuckets {
  constructor(public data: JobSample[][],
              public start: number,
              public end: number,
              public max: number) { }

  public limitMaximum(maximum: number) {
    if (this.max > maximum) {
        this.max = maximum;
    }
  }

  public linearChunks(bucket: JobSample[], rows: number): JobSample[][] {
      const stride = Math.ceil((this.max) / rows);
      const chunks: JobSample[][] = [];
      chunks[0] = [];
      let next = stride;
      for (const sample of bucket) {
          if (sample.duration <= next) {
              chunks[chunks.length - 1].push(sample);
              continue;
          }
          next = next + stride;
          while (next < sample.duration) {
            if (chunks.length > (rows - 1)) {
                break;
            }
            chunks.push([]);
            next = next + stride;
          }
          if (chunks.length > (rows - 1)) {
            chunks[chunks.length - 1].push(sample);
        } else {
            chunks.push([sample]);
          }
      }
      if (chunks.length > rows) {
        throw new Error("invalid rows");
      }
      return chunks;
  }
}
