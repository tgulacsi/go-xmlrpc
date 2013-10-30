#!/usr/bin/env python
# -*- coding: utf-8 -*-

import xmlrpclib


def main(url, method_name, *args):
    server = xmlrpclib.ServerProxy(url, verbose=True, allow_none=True,
            use_datetime=True)
    print('calling %s(%r) at %s' % (method_name, args, url))
    print(getattr(server, method_name)(*map(int, args)))

if '__main__' == __name__:
    import sys
    main(*sys.argv[1:])
