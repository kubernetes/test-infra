# tags appends default tags to name
#
# In particular, names is a {image_prefix: image_target} mapping, which gets
# expanded into three full image paths:
#   image_prefix:latest
#   image_prefix:latest-{BUILD_USER}
#   image_prefix:{DOCKER_TAG}
# (See print-workspace-status.sh for how BUILD_USER and DOCKER_TAG are created.
#
# Concretely, tags(this=":that-image", foo="//bar") will return:
#   {
#     "this:latest": ":that-image",
#     "this:latest-fejta": ":that-image",
#     "this:20180203-deadbeef": ":that-image",
#     "foo:latest": "//bar",
#     "foo:latest-fejta": "//bar",
#     "foo:20180203-deadbeef", "//bar",
#   }
def tags(**names):
  outs = {}
  for img, target in names.items():
    outs['%s:{DOCKER_TAG}' % img] = target
    outs['%s:latest-{BUILD_USER}' % img] = target
    outs['%s:latest' % img] = target
  return outs
