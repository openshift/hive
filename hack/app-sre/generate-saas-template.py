#!/usr/bin/env python

import os
import sys
import yaml

usage = """
Usage: %s saas_template_stub saas_object_file out_file

Collate `saas_object_file` into the objects list in `saas_template_stub` and
write the result to `out_file`.

Parameters:
  saas_template_stub: A yaml file defining a Template with an empty `objects`
        list.
  saas_object_file: A yaml file containing some number of yaml documents
        defining kube objects to be injected into the `saas_template_stub`.
  out_file: Path to which to write the resulting yaml file. If it exists, it
        will be overwritten.
"""

if len(sys.argv) != 4 or any("-h" in x for x in sys.argv[1:]):
    print(usage % sys.argv[0])
    sys.exit(-1)

saas_template_stub = sys.argv[1]
saas_object_file = sys.argv[2]
out_file = sys.argv[3]

for f in (saas_template_stub, saas_object_file):
    if not os.path.isfile(f):
        print("%s: No such file" % f)

if os.path.exists(out_file) and not os.path.isfile(out_file):
    print("%s: Not a file" % out_file)
    sys.exit(1)

with open(saas_object_file, "r") as objf:
    objects = list(yaml.load_all(objf, Loader=yaml.SafeLoader))
    print("Loaded %d objects from %s" % (len(objects), saas_object_file))

with open(saas_template_stub, "r") as stubf:
    template = yaml.load(stubf, Loader=yaml.SafeLoader)

template["objects"] = objects

msg = "Overwriting existing" if os.path.exists(out_file) else "Writing"
print(msg + " output file %s" % out_file)

with open(out_file, "w") as outf:
    yaml.dump(template, outf, default_flow_style=False)
