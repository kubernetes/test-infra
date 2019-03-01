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
