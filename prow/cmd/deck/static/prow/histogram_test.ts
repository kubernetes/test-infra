import "jasmine";
import {JobHistogram, JobSample} from "./histogram";

describe('JobHistogram', () => {
  it('should do nothing with empty input', () => {
    const h = new JobHistogram();
    const buckets = h.buckets(0, 9, 1);
    expect(buckets.data.length).toEqual(1);
    expect(buckets.data[0].length).toEqual(0);
  });
  it('should filter entries outside start end', () => {
    const h = new JobHistogram();
    h.add(new JobSample(10, 100, "failure", 0));
    h.add(new JobSample(10, 100, "success", 1));
    h.add(new JobSample(200, 10, "failure", 2));
    h.add(new JobSample(201, 10, "failure", -1));
    expect(h.buckets(0, 9, 1).data).toEqual([[]]);
    expect(h.buckets(0, 9, 2).data).toEqual([[], []]);
  });
  it('should create chunks for each bucket', () => {
    const h = new JobHistogram();
    const samples = [
      new JobSample(10, 100, "failure", 0),
      new JobSample(10, 101, "success", 1),
      new JobSample(300, 200, "success", 2),
      new JobSample(200, 300, "success", 3),
      new JobSample(200, 300, "success", 4),
      new JobSample(0, 500, "success", 5),
      new JobSample(200, 11, "failure", 6),
      new JobSample(201, 10, "failure", 7),
    ];
    for (const sample of samples) {
      h.add(sample);
    }
    const buckets = h.buckets(0, 1000, 10);
    expect(buckets.linearChunks(buckets.data[0], 4)).toEqual([
      [samples[0], samples[1]],
      [],
      [],
      [samples[5]],
    ]);
    expect(buckets.linearChunks(buckets.data[1], 4)).toEqual([
      [],
      [],
      [samples[3]],
    ]);
    expect(buckets.linearChunks(buckets.data[2], 4)).toEqual([
      [samples[7], samples[6]],
      [],
      [samples[4]],
    ]);
    expect(buckets.linearChunks(buckets.data[3], 4)).toEqual([
      [],
      [samples[2]],
    ]);
  });
  it('should create limited chunks for each bucket', () => {
    const h = new JobHistogram();
    const samples = [
      new JobSample(10, 100, "failure", 0),
      new JobSample(10, 101, "success", 1),
      new JobSample(300, 200, "success", 2),
      new JobSample(200, 300, "success", 3),
      new JobSample(200, 300, "success", 4),
      new JobSample(0, 500, "success", 5),
      new JobSample(200, 11, "failure", 6),
      new JobSample(201, 10, "failure", 7),
    ];
    for (const sample of samples) {
      h.add(sample);
    }
    const buckets = h.buckets(0, 1000, 10);
    buckets.limitMaximum(300);
    expect(buckets.linearChunks(buckets.data[0], 4)).toEqual([
      [],
      [samples[0], samples[1]],
      [],
      [samples[5]],
    ]);
    expect(buckets.linearChunks(buckets.data[1], 4)).toEqual([
      [],
      [],
      [],
      [samples[3]],
    ]);
    expect(buckets.linearChunks(buckets.data[2], 4)).toEqual([
      [samples[7], samples[6]],
      [],
      [],
      [samples[4]],
    ]);
    expect(buckets.linearChunks(buckets.data[3], 4)).toEqual([
      [],
      [],
      [samples[2]],
    ]);
  });
  it('should create buckets that contain the correct results', () => {
    const h = new JobHistogram();
    const samples = [
      new JobSample(10, 100, "failure", 0),
      new JobSample(10, 101, "success", 1),
      new JobSample(200, 11, "failure", 2),
      new JobSample(201, 10, "failure", -1),
    ];
    for (const sample of samples) {
      h.add(sample);
    }
    expect(h.buckets(0, 10, 1).data).toEqual([
      [samples[0], samples[1]],
    ]);
    expect(h.buckets(0, 11, 1).data).toEqual([
      [samples[0], samples[1]],
    ]);
    expect(h.buckets(199, 200, 1).data).toEqual([
      [samples[2]],
    ]);
    expect(h.buckets(0, 201, 2).data).toEqual([
      [samples[0], samples[1]],
      [samples[3], samples[2]],
    ]);
    expect(h.buckets(0, 202, 2).data).toEqual([
      [samples[0], samples[1]],
      [samples[3], samples[2]],
    ]);

    const swap = samples[0];
    samples[0] = samples[1];
    samples[1] = swap;
    expect(h.buckets(0, 202, 2).data).toEqual([
      [samples[1], samples[0]],
      [samples[3], samples[2]],
    ]);
  });
});
