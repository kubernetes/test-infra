def _http_archive_with_pkg_path_impl(ctx):
    """Implements the http_archive_with_pkg_path build rule."""
    ctx.execute(["mkdir", "-p", ctx.attr.pkg_path])
    ctx.download_and_extract(
        url=ctx.attr.url,
        sha256=ctx.attr.sha256,
        stripPrefix=ctx.attr.strip_prefix,
        output=ctx.attr.pkg_path)
    ctx.file(ctx.attr.pkg_path+"/BUILD.bazel", ctx.attr.build_file_content)

# http_archive_with_pkg_path extends the built-in new_http_archive with a
# pkg_path field, which can be used to specify the package installation path.
http_archive_with_pkg_path = repository_rule(
    implementation = _http_archive_with_pkg_path_impl,
    attrs = {
        "build_file_content": attr.string(mandatory=True),
        "pkg_path": attr.string(mandatory=True),
        "sha256": attr.string(),
        "strip_prefix": attr.string(mandatory=True),
        "url": attr.string(mandatory=True),
    },
)
