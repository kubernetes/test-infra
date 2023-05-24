"""
Helper rules for language proto.
"""

def _generate_tables_impl(ctx):
    args = ctx.actions.args()
    args.add("-input", ctx.file.src)
    args.add("-output", ctx.outputs.out)
    ctx.actions.run(
        executable = ctx.executable.bin,
        inputs = [ctx.file.src],
        outputs = [ctx.outputs.out],
        arguments = [args],
    )

generate_tables = rule(
    implementation = _generate_tables_impl,
    attrs = {
        "src": attr.label(allow_single_file = True),
        "out": attr.output(),
        "bin": attr.label(
            default = "//generatetables",
            executable = True,
            allow_single_file = True,
            cfg = "host",
        ),
    },
)
