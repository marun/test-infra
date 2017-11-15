load("@io_bazel_rules_go//go:def.bzl", "go_library")

go_library(
    name = "go_default_library",
    srcs = [
        "milestone-maintainer.go",
        "notification.go",
        "target.go",
    ],
    importpath = "k8s.io/test-infra/prow/plugins/milestonemaintainer",
    visibility = ["//visibility:public"],
    deps = [
        "//mungegithub/mungers/approvers:go_default_library",
        "//mungegithub/mungers/matchers/comment:go_default_library",
        "//prow/github:go_default_library",
        "//prow/plugins:go_default_library",
        "//vendor/github.com/sirupsen/logrus:go_default_library",
        "//vendor/k8s.io/kubernetes/pkg/util/sets:go_default_library",
    ],
)

filegroup(
    name = "package-srcs",
    srcs = glob(["**"]),
    tags = ["automanaged"],
    visibility = ["//visibility:private"],
)

filegroup(
    name = "all-srcs",
    srcs = [":package-srcs"],
    tags = ["automanaged"],
    visibility = ["//visibility:public"],
)
