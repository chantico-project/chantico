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

group "default" {
  targets = ["controller", "visualisation-webapp", "snmp-mock", "vm-attribution-mock"]
}

group "mocks" {
  targets = ["vm-attribution-mock", "snmp-mock"]
}

target "controller" {
  context    = "."
  dockerfile = "Dockerfile"
  tags = concat(
    ["${IMAGE_REGISTRY}/chantico${IMAGE_SUFFIX}:${IMAGE_TAG}"],
    IMAGE_SHA_TAG != "" ? ["${IMAGE_REGISTRY}/chantico${IMAGE_SUFFIX}:${IMAGE_SHA_TAG}"] : []
  )
}

target "visualisation-webapp" {
  context    = "."
  dockerfile = "Dockerfile.webapp"
  tags = concat(
    ["${IMAGE_REGISTRY}/chantico-visualisation-webapp${IMAGE_SUFFIX}:${IMAGE_TAG}"],
    IMAGE_SHA_TAG != "" ? ["${IMAGE_REGISTRY}/chantico-visualisation-webapp${IMAGE_SUFFIX}:${IMAGE_SHA_TAG}"] : []
  )
}

target "snmp-mock" {
  context    = "."
  dockerfile = "Dockerfile.snmp-mock"
  tags = concat(
    ["${IMAGE_REGISTRY}/chantico-snmp-mock${IMAGE_SUFFIX}:${IMAGE_TAG}"],
    IMAGE_SHA_TAG != "" ? ["${IMAGE_REGISTRY}/chantico-snmp-mock${IMAGE_SUFFIX}:${IMAGE_SHA_TAG}"] : []
  )
}

target "vm-attribution-mock" {
  context    = "."
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
