import gptDirect from './gptDirect'
import accountPerformance from './accountPerformance'
import coupon from './coupon'
import invoice from './invoice'
import billingRules from './billingRules'
import optimizations from './optimizations'
import { homeMessages } from '../../homeOverview'
import accountTestTiming from './accountTestTiming'
import openaiAuth from './openaiAuth'
import providerHub from './providerHub'
import traffic from './traffic'
import connections from './connections'
import siwc from './siwc'
import qualityOps from './qualityOps'
import modelTrace from './modelTrace'
import adminAccessKeys from './adminAccessKeys'
import mcp from './mcp'
import landing from './landing'
import common from './common'
import dashboard from './dashboard'
import channelMonitorV2 from './channelMonitorV2'
import channelMonitorV3 from './channelMonitorV3'
import supportTickets from './supportTickets'
import batchImage from './batchImage'
import admin from './admin'
import misc from './misc'
import native from './native'
import videoModule from './videoModule'

import requestTiming from './requestTiming'


import qqbot from './qqbot'

export default {
  gptDirect,
  accountPerformance,
  coupon,
  invoice,
  billingRules,
  ...qqbot,
  homeOverview: homeMessages.en,
  accountTestTiming,
  openaiAuth,
  traffic,
  connections,
  siwc,
  qualityOps,
  providerHub,
  modelTrace,
  requestTiming,
  adminAccessKeys,
  mcp,
  ...landing,
  ...common,
  ...dashboard,
  ...channelMonitorV2,
  ...channelMonitorV3,
  ...supportTickets,
  ...batchImage,
  admin,
  ...misc,
  ...native,
  videoModule,
  optimizations,
}
