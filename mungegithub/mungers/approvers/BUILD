package(default_visibility = ["//visibility:public"])

licenses(["notice"])

load(
    "@io_bazel_rules_go//go:def.bzl",
    "go_library",
    "go_test",
)

go_test(
    name = "go_default_test",
    srcs = [
        "approvers_test.go",
        "owners_test.go",
    ],
    library = ":go_default_library",
    tags = ["automanaged"],
    deps = ["//vendor:k8s.io/kubernetes/pkg/util/sets"],
)

go_library(
    name = "go_default_library",
    srcs = ["owners.go"],
    tags = ["automanaged"],
    deps = [
        "//mungegithub/mungers/matchers/comment:go_default_library",
        "//vendor:github.com/golang/glog",
        "//vendor:k8s.io/kubernetes/pkg/util/sets",
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
)
