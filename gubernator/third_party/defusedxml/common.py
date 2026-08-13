# defusedxml
#
# Copyright (c) 2013 by Christian Heimes <christian@python.org>
# Licensed to PSF under a Contributor Agreement.
# See http://www.python.org/psf/license for licensing details.
"""Common constants, exceptions and helpe functions
"""
import sys
from types import MethodType

PY3 = sys.version_info[0] == 3
PY26 = sys.version_info[:2] == (2, 6)
PY31 = sys.version_info[:2] == (3, 1)


class DefusedXmlException(ValueError):
    """Base exception
    """
    def __repr__(self):
        return str(self)


class DTDForbidden(DefusedXmlException):
    """Document type definition is forbidden
    """
    def __init__(self, name, sysid, pubid):
        super(DTDForbidden, self).__init__()
        self.name = name
        self.sysid = sysid
        self.pubid = pubid

    def __str__(self):
        tpl = "DTDForbidden(name='{}', system_id={!r}, public_id={!r})"
        return tpl.format(self.name, self.sysid, self.pubid)


class EntitiesForbidden(DefusedXmlException):
    """Entity definition is forbidden
    """
    def __init__(self, name, value, base, sysid, pubid, notation_name):
        super(EntitiesForbidden, self).__init__()
        self.name = name
        self.value = value
        self.base = base
        self.sysid = sysid
        self.pubid = pubid
        self.notation_name = notation_name

    def __str__(self):
        tpl = "EntitiesForbidden(name='{}', system_id={!r}, public_id={!r})"
        return tpl.format(self.name, self.sysid, self.pubid)


class ExternalReferenceForbidden(DefusedXmlException):
    """Resolving an external reference is forbidden
    """
    def __init__(self, context, base, sysid, pubid):
        super(ExternalReferenceForbidden, self).__init__()
        self.context = context
        self.base = base
        self.sysid = sysid
        self.pubid = pubid

    def __str__(self):
        tpl = "ExternalReferenceForbidden(system_id='{}', public_id={})"
        return tpl.format(self.sysid, self.pubid)


class NotSupportedError(DefusedXmlException):
    """The operation is not supported
    """


def _apply_defusing(defused_mod):
    assert defused_mod is sys.modules[defused_mod.__name__]
    stdlib_name = defused_mod.__origin__
    __import__(stdlib_name, {}, {}, ["*"])
    stdlib_mod = sys.modules[stdlib_name]
    stdlib_names = set(dir(stdlib_mod))
    for name, obj in vars(defused_mod).items():
        if name.startswith("_") or name not in stdlib_names:
            continue
        setattr(stdlib_mod, name, obj)
    return stdlib_mod


def _generate_etree_functions(DefusedXMLParser, _TreeBuilder,
            _IterParseIterator, _parse, _iterparse):
    """Factory for functions needed by etree, dependent on whether
    cElementTree or ElementTree is used."""

    def parse(source, parser=None, forbid_dtd=False, forbid_entities=True,
              forbid_external=True):
        if parser is None:
            parser = DefusedXMLParser(target=_TreeBuilder(),
                                      forbid_dtd=forbid_dtd,
                                      forbid_entities=forbid_entities,
                                      forbid_external=forbid_external)
        return _parse(source, parser)

    if PY26 or PY31:
        def bind(xmlparser, funcname, hookname):
            func = getattr(DefusedXMLParser, funcname)
            if PY26:
                # unbound -> function
                func = func.__func__
                method = MethodType(func, xmlparser, xmlparser.__class__)
            else:
                method = MethodType(func, xmlparser)
            # set hook
            setattr(xmlparser._parser, hookname, method)

        def iterparse(source, events=None, forbid_dtd=False,
                      forbid_entities=True, forbid_external=True):
            it = _iterparse(source, events)
            xmlparser = it._parser
            if forbid_dtd:
                bind(xmlparser, "defused_start_doctype_decl",
                     "StartDoctypeDeclHandler")
            if forbid_entities:
                bind(xmlparser, "defused_entity_decl",
                     "EntityDeclHandler")
                bind(xmlparser, "defused_unparsed_entity_decl",
                     "UnparsedEntityDeclHandler")
            if forbid_external:
                bind(xmlparser, "defused_external_entity_ref_handler",
                     "ExternalEntityRefHandler")
            return it
    elif PY3:
        def iterparse(source, events=None, parser=None, forbid_dtd=False,
                      forbid_entities=True, forbid_external=True):
            close_source = False
            if not hasattr(source, "read"):
                source = open(source, "rb")
                close_source = True
            if not parser:
                parser = DefusedXMLParser(target=_TreeBuilder(),
                                          forbid_dtd=forbid_dtd,
                                          forbid_entities=forbid_entities,
                                          forbid_external=forbid_external)
            return _IterParseIterator(source, events, parser, close_source)
    else:
        # Python 2.7
        def iterparse(source, events=None, parser=None, forbid_dtd=False,
                      forbid_entities=True, forbid_external=True):
            if parser is None:
                parser = DefusedXMLParser(target=_TreeBuilder(),
                                          forbid_dtd=forbid_dtd,
                                          forbid_entities=forbid_entities,
                                          forbid_external=forbid_external)
            return _iterparse(source, events, parser)

    def fromstring(text, forbid_dtd=False, forbid_entities=True,
                   forbid_external=True):
        parser = DefusedXMLParser(target=_TreeBuilder(),
                                  forbid_dtd=forbid_dtd,
                                  forbid_entities=forbid_entities,
                                  forbid_external=forbid_external)
        parser.feed(text)
        return parser.close()


    return parse, iterparse, fromstring
