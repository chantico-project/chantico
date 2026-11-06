variable "IMAGE_REGISTRY" {
  default = "ghcr.io/chantico-project/images"
}

variable "IMAGE_SUFFIX" {
  default = ""
}

variable "IMAGE_TAG" {
  default = "latest"
}

variable "IMAGE_SHA_TAG" {
  default = ""
}

# Comma-separated target architectures, e.g. "amd64,arm64". Empty builds for the host architecture.
variable "IMAGE_ARCH" {
  default = ""
}

group "default" {
  targets = ["controller", "visualisation-webapp", "snmp-mock", "vm-attribution-mock"]
}

group "mocks" {
  targets = ["vm-attribution-mock", "snmp-mock"]
}

target "_common" {
  context   = "."
  platforms = [for arch in compact(split(",", IMAGE_ARCH)) : "linux/${trimspace(arch)}"]
}

target "controller" {
  inherits   = ["_common"]
  dockerfile = "Dockerfile"
  tags = concat(
    ["${IMAGE_REGISTRY}/chantico${IMAGE_SUFFIX}:${IMAGE_TAG}"],
    IMAGE_SHA_TAG != "" ? ["${IMAGE_REGISTRY}/chantico${IMAGE_SUFFIX}:${IMAGE_SHA_TAG}"] : []
  )
}

target "visualisation-webapp" {
  inherits   = ["_common"]
  dockerfile = "Dockerfile.webapp"
  tags = concat(
    ["${IMAGE_REGISTRY}/chantico-visualisation-webapp${IMAGE_SUFFIX}:${IMAGE_TAG}"],
    IMAGE_SHA_TAG != "" ? ["${IMAGE_REGISTRY}/chantico-visualisation-webapp${IMAGE_SUFFIX}:${IMAGE_SHA_TAG}"] : []
  )
}

target "snmp-mock" {
  inherits   = ["_common"]
  dockerfile = "Dockerfile.snmp-mock"
  tags = concat(
    ["${IMAGE_REGISTRY}/chantico-snmp-mock${IMAGE_SUFFIX}:${IMAGE_TAG}"],
    IMAGE_SHA_TAG != "" ? ["${IMAGE_REGISTRY}/chantico-snmp-mock${IMAGE_SUFFIX}:${IMAGE_SHA_TAG}"] : []
  )
}

target "vm-attribution-mock" {
  inherits   = ["_common"]
  dockerfile = "Dockerfile.vm-attribution-mock"
  tags = concat(
    ["${IMAGE_REGISTRY}/chantico-vm-attribution-mock${IMAGE_SUFFIX}:${IMAGE_TAG}"],
    IMAGE_SHA_TAG != "" ? ["${IMAGE_REGISTRY}/chantico-vm-attribution-mock${IMAGE_SUFFIX}:${IMAGE_SHA_TAG}"] : []
  )
}

# CI-only targets that add GitHub Actions cache, e.g. ci-controller.
target "ci" {
  name     = "ci-${tgt}"
  matrix   = { tgt = ["controller", "visualisation-webapp", "snmp-mock", "vm-attribution-mock"] }
  inherits = [tgt]
  cache-from = ["type=gha,scope=${tgt}"]
  cache-to   = ["type=gha,scope=${tgt},mode=max,ignore-error=true"]
}
