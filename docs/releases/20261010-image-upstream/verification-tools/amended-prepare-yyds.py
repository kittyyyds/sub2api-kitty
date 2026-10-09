import copy,sys
sys.dont_write_bytecode=True
sys.path.insert(0,'/opt/kitty-recovery-20261009/image-upstream-release-r2')
import release_common as rc
amendment_file=rc.BASE/'yyds-price-amendment.json'
rc.require(rc.sha(amendment_file)=='02b5a4398dcb06de5ea4011b8057d15910d4c73c9d57d4aca54f99b07b9a1655','Reviewed price amendment changed')
amendment=rc.read_json(amendment_file)
guard=rc.data_guard
def amended_guard(site,c):
    if site=='kittyyyds':
        rc.require(amendment['identity']==c['identity'],'Amendment build mismatch')
        c=copy.deepcopy(c)
        c['preflight']['sites'][site]['video_price_digest']=amendment['video_price_digest']
        c['preflight']['sites'][site]['protected']=amendment['protected']
    return guard(site,c)
rc.data_guard=amended_guard
rc.prepare('kittyyyds')
